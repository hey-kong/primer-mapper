package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"syscall"
	"time"

	logger "github.com/d2r2/go-logger"
	"github.com/d2r2/go-shell"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"primer-mapper/common"
)

var log = logger.NewPackageLogger("main",
	logger.DebugLevel,
	// logger.InfoLevel,
)

func connectToMqtt() mqtt.Client {
	opts := mqtt.NewClientOptions().AddBroker("mqtt://175.178.163.249:1883")

	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	return c
}

func main() {
	defer logger.FinalizeLogger()

	log.Notify("***************************************************************************************************")
	log.Notify("*** You can change verbosity of output, to modify logging level of module \"dht\"")
	log.Notify("*** Uncomment/comment corresponding lines with call to ChangePackageLogLevel(...)")
	log.Notify("***************************************************************************************************")
	log.Notify("*** Massive stress test of sensor reading, printing in the end summary statistical results")
	log.Notify("***************************************************************************************************")
	// Uncomment/comment next line to suppress/increase verbosity of output
	logger.ChangePackageLogLevel("dht", logger.InfoLevel)

	// create context with cancellation possibility
	ctx, cancel := context.WithCancel(context.Background())
	// use done channel as a trigger to exit from signal waiting goroutine
	done := make(chan struct{})
	defer close(done)
	// build actual signal list to control
	signals := []os.Signal{os.Kill, os.Interrupt}
	if shell.IsLinuxMacOSFreeBSD() {
		signals = append(signals, syscall.SIGTERM)
	}
	// run goroutine waiting for OS termination events, including keyboard Ctrl+C
	shell.CloseContextOnSignals(cancel, done, signals...)

	term := false

	// connect to Mqtt broker
	cli := connectToMqtt()

	// subscribe device msg
	handleZigbee(cli)

	// TODO: subscribe command msg

	for {
		select {
		// Check for termination request
		case <-ctx.Done():
			log.Errorf("Termination pending: %s", ctx.Err())
			term = true
			// sleep 10 ms before next round
			// (recommended by specification as "collecting period")
		case <-time.After(1000 * time.Millisecond):
		}
		if term {
			break
		}
	}
	log.Info("exited")
}

var OperateUpdateZigbeeSub mqtt.MessageHandler = func(cli mqtt.Client, msg mqtt.Message) {
	log.Info("Receive msg\n", string(msg.Payload()))
	current := make(map[string]string)
	if err := json.Unmarshal(msg.Payload(), &current); err != nil {
		log.Errorf("unmarshal receive msg to device state, error %v\n", err)
		return
	}

	// publish status to mqtt broker
	publishToMqtt(cli, current)
}

func handleZigbee(cli mqtt.Client) {
	topic := "zigbee/+"
	cli.Subscribe(topic, 0, OperateUpdateZigbeeSub)
}

func publishToMqtt(cli mqtt.Client, current map[string]string) {
	id := current["id"]
	if id == "" {
		return
	}

	deviceTwinUpdate := common.DevicePrefix + id + common.TwinUpdateSuffix
	timestamp := common.GetTimestamp()
	for f, v := range current {
		go func(field string, value string) {
			updateMessage := createActualUpdateMessage(field, value, timestamp)
			twinUpdateBody, _ := json.Marshal(updateMessage)
			fmt.Println(string(twinUpdateBody))

			cli.Publish(deviceTwinUpdate, 0, true, twinUpdateBody)
		}(f, v)
	}
}

//createActualUpdateMessage function is used to create the device twin update message
func createActualUpdateMessage(field string, value string, timestamp int64) common.DeviceTwinUpdate {
	var updateMsg common.DeviceTwinUpdate
	updateMsg.BaseMessage.Timestamp = timestamp
	updateMsg.Twin = map[string]*common.MsgTwin{}
	updateMsg.Twin[field] = &common.MsgTwin{}
	updateMsg.Twin[field].Actual = &common.TwinValue{Value: &value}
	updateMsg.Twin[field].Metadata = &common.TypeMetadata{Type: reflect.TypeOf(value).String()}
	return updateMsg
}
