package main

import (
	"context"
	"encoding/json"
	"os"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/d2r2/go-shell"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"

	"primer-mapper/common"
)

var mqttServer = "mqtt://175.178.163.249:1883"

var boltDB *bolt.DB
var dir = "/tmp/boltdb"
var dbName = "boltdb.db"
var tblName = "DeviceStatus"

var c *cache.Cache
var expiration = 60 * time.Second

var logFile = "/tmp/log/primer_mapper.log"
var log = logrus.New()

func connectToMqtt() mqtt.Client {
	opts := mqtt.NewClientOptions().AddBroker(mqttServer)

	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	return c
}

func main() {
	// create or load boltDB to persist device status
	loadDB()
	defer boltDB.Close()

	// create a cache with an expiration time , and which purges expired
	// items every 2*expiration time for checking offline devices
	c = cache.New(expiration, 2*expiration)
	c.OnEvicted(updateToOfflineStatus)

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.Out = file
	} else {
		log.Info("Failed to log to file, using default stderr")
	}
	log.Println("***************************************************************************************************")
	log.Println("*** You can change verbosity of output, to modify logging level of module \"dht\"")
	log.Println("*** Uncomment/comment corresponding lines with call to ChangePackageLogLevel(...)")
	log.Println("***************************************************************************************************")
	log.Println("*** Massive stress test of sensor reading, printing in the end summary statistical results")
	log.Println("***************************************************************************************************")

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
		case <-time.After(10 * time.Millisecond):
		}
		if term {
			break
		}
	}
	log.Info("exited")
}

var OperateUpdateZigbeeSub mqtt.MessageHandler = func(cli mqtt.Client, msg mqtt.Message) {
	log.Info("Receive msg: ", string(msg.Payload()))
	current := make(map[string]interface{})
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

func publishToMqtt(cli mqtt.Client, current map[string]interface{}) {
	id := current["id"].(string)
	if id == "" {
		return
	}

	// check status
	c.Set(id, &cli, cache.DefaultExpiration)
	updateToOnlineStatus(cli, id)
	// forward message
	deviceTwinUpdate := common.DevicePrefix + id + common.TwinUpdateSuffix
	t := common.GetTimestamp()
	for f, v := range current {
		if f == "id" {
			continue
		}
		go func(field string, value interface{}, timestamp int64) {
			updateMessage := createActualUpdateMessage(field, common.Itos(value), timestamp)
			twinUpdateBody, _ := json.Marshal(updateMessage)

			cli.Publish(deviceTwinUpdate, 0, true, twinUpdateBody)
		}(f, v, t)
	}
}

//createActualUpdateMessage function is used to create the device twin update message
func createActualUpdateMessage(field string, value string, timestamp int64) common.DeviceTwinUpdate {
	var updateMsg common.DeviceTwinUpdate
	updateMsg.BaseMessage.Timestamp = timestamp
	updateMsg.Twin = map[string]*common.MsgTwin{}
	updateMsg.Twin[field] = &common.MsgTwin{}
	updateMsg.Twin[field].Actual = &common.TwinValue{Value: &value}
	updateMsg.Twin[field].Metadata = &common.TypeMetadata{Type: "string"}
	return updateMsg
}

func loadDB() {
	if ok := common.PathIsExist(dir); !ok {
		common.CreateDir(dir)
	}

	var err error
	boltDB, err = bolt.Open(dir+dbName, 0666, nil)
	if err != nil {
		panic(err)
	}

	err = boltDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tblName))
		if b == nil {
			_, err := tx.CreateBucket([]byte(tblName))
			if err != nil {
				log.Fatal(err)
			}
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

var inactive = ""
var online = "online"
var offline = "offline"
var disabled = "disabled"

func updateToOnlineStatus(cli mqtt.Client, deviceID string) {
	tx, err := boltDB.Begin(true)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	b := tx.Bucket([]byte(tblName))
	if b != nil {
		val := b.Get([]byte(deviceID))
		status := string(val)
		if status == inactive || status == offline {
			deviceTwinUpdate := common.DevicePrefix + deviceID + common.TwinUpdateSuffix
			updateMessage := createActualUpdateMessage("status", online, common.GetTimestamp())
			twinUpdateBody, _ := json.Marshal(updateMessage)
			log.Printf("device %s is online", deviceID)

			cli.Publish(deviceTwinUpdate, 0, true, twinUpdateBody)
			if err := b.Put([]byte(deviceID), []byte(online)); err != nil {
				log.Fatal(err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
}

func updateToOfflineStatus(deviceID string, v interface{}) {
	tx, err := boltDB.Begin(true)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	b := tx.Bucket([]byte(tblName))
	if b != nil {
		cli := *v.(*mqtt.Client)
		deviceTwinUpdate := common.DevicePrefix + deviceID + common.TwinUpdateSuffix
		updateMessage := createActualUpdateMessage("status", offline, common.GetTimestamp())
		twinUpdateBody, _ := json.Marshal(updateMessage)
		log.Printf("device %s is offline", deviceID)

		cli.Publish(deviceTwinUpdate, 0, true, twinUpdateBody)
		if err := b.Put([]byte(deviceID), []byte(offline)); err != nil {
			log.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
}
