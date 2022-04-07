package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

	"primer-mapper/common"
	"primer-mapper/device"

	logger "github.com/d2r2/go-logger"
	"github.com/d2r2/go-shell"
	"github.com/yosssi/gmq/mqtt"
	"github.com/yosssi/gmq/mqtt/client"
)

var log = logger.NewPackageLogger("main",
	logger.DebugLevel,
	// logger.InfoLevel,
)

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

	for {
		topic := "Zigbee/0ea11c12123a"
		err := cli.Subscribe(&client.SubscribeOptions{
			SubReqs: []*client.SubReq{
				&client.SubReq{
					TopicFilter: []byte(topic),
					QoS:         mqtt.QoS0,
					Handler: func(topicName, message []byte) {
						OperateUpdateZigbeeSub(cli, message)
					},
				},
			},
		})

		if err != nil {
			panic(err)
		}

		select {
		// Check for termination request
		case <-ctx.Done():
			log.Errorf("Termination pending: %s", ctx.Err())
			term = true
			// sleep 100 ms before next round
			// (recommended by specification as "collecting period")
		case <-time.After(100 * time.Millisecond):
		}
		if term {
			break
		}
	}
	log.Info("exited")
}

func connectToMqtt() *client.Client {
	cli := client.New(&client.Options{
		// Define the processing of the error handler.
		ErrorHandler: func(err error) {
			fmt.Println(err)
		},
	})
	defer cli.Terminate()

	// Connect to the MQTT Server.
	err := cli.Connect(&client.ConnectOptions{
		Network:  "tcp",
		Address:  "localhost:1883",
		ClientID: []byte("receive-client"),
	})
	if err != nil {
		panic(err)
	}
	return cli
}

func OperateUpdateZigbeeSub(cli *client.Client, msg []byte) {
	log.Info("Receive msg\n", string(msg))
	current := &device.ZigbeeDeviceState{}
	if err := json.Unmarshal(msg, current); err != nil {
		log.Errorf("unmarshal receive msg to device state, error %v\n", err)
		return
	}

	// publish status to mqtt broker
	valueMap := map[string]string{"temperature": strconv.Itoa(current.Temperature), "humidity": strconv.Itoa(current.Humidity), "led": strconv.FormatBool(current.Led)}
	publishToMqtt(cli, valueMap)
}

func publishToMqtt(cli *client.Client, valueMap map[string]string) {
	deviceTwinUpdate := common.DevicePrefix + common.DeviceID + common.TwinUpdateSuffix

	for f, v := range valueMap {
		updateMessage := createActualUpdateMessage(f, v)
		twinUpdateBody, _ := json.Marshal(updateMessage)
		log.Info(string(twinUpdateBody))

		cli.Publish(&client.PublishOptions{
			TopicName: []byte(deviceTwinUpdate),
			QoS:       mqtt.QoS0,
			Message:   twinUpdateBody,
		})
	}
}

//createActualUpdateMessage function is used to create the device twin update message
func createActualUpdateMessage(field string, actualValue string) common.DeviceTwinUpdate {
	var deviceTwinUpdateMessage common.DeviceTwinUpdate
	actualMap := map[string]*common.MsgTwin{field: {Actual: &common.TwinValue{Value: &actualValue}, Metadata: &common.TypeMetadata{Type: "Updated"}}}
	deviceTwinUpdateMessage.Twin = actualMap
	return deviceTwinUpdateMessage
}
