package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"primer-mapper/api"
	"strconv"
	"strings"
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
var dir = "/tmp/boltdb/"
var dbName = "boltdb.db"
var statusTblName = "DeviceStatus"
var gatewayTblName = "DeviceGate"

var c *cache.Cache
var expiration = 60 * time.Second

var logFile = "/tmp/log/primer_mapper.log"
var log = logrus.New()

var inactive = "offactivate"
var online = "online"
var offline = "offline"
var disabled = "disabled"

func connectToMqtt() mqtt.Client {
	opts := mqtt.NewClientOptions().AddBroker(mqttServer)

	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(1 * time.Second)

	cli := mqtt.NewClient(opts)
	if token := cli.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	return cli
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
		b := tx.Bucket([]byte(statusTblName))
		if b == nil {
			_, err = tx.CreateBucket([]byte(statusTblName))
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("create bucket %s", statusTblName)
		}
		b = tx.Bucket([]byte(gatewayTblName))
		if b == nil {
			_, err = tx.CreateBucket([]byte(gatewayTblName))
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("create bucket %s", gatewayTblName)
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	// create or load boltDB to persist device status
	loadDB()
	defer boltDB.Close()

	// create a cache with an expiration time, and which purges expired
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
	handleWifi(cli)
	handleBluetooth(cli)
	handleNBIoT(cli)

	// subscribe command msg
	handleDownstream(cli)

	// subscribe membership msg
	handleMembership(cli)
	handleMembershipResult(cli)

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

// OperateUpdateUpstream function is used to process uplink device message
var OperateUpdateUpstream mqtt.MessageHandler = func(cli mqtt.Client, msg mqtt.Message) {
	log.Info("Receive msg: ", string(msg.Payload()))
	current := make(map[string]interface{})
	if err := json.Unmarshal(msg.Payload(), &current); err != nil {
		log.Errorf("unmarshal receive msg to device state, error %v\n", err)
		return
	}

	// publish to mqtt broker
	publishToMqtt(cli, current, false)
}

// OperateUpdateDownstream function is used to process downlink device message
var OperateUpdateDownstream mqtt.MessageHandler = func(cli mqtt.Client, msg mqtt.Message) {
	id := common.GetDeviceID(msg.Topic())
	if id == "" {
		log.Fatal("Wrong topic")
		return
	}

	log.Info("Receive msg: ", string(msg.Payload()))
	var delta common.DeviceTwinDelta
	if err := json.Unmarshal(msg.Payload(), &delta); err != nil {
		log.Errorf("Unmarshal message failed: %v", err)
		return
	}

	upstream := make(map[string]interface{})
	downstream := make(map[string]interface{})
	for twinName, twinValue := range delta.Delta {
		if twinName == disabled {
			status := disabled
			if twinValue == "0" {
				status = inactive
			}

			tx, err := boltDB.Begin(true)
			if err != nil {
				log.Fatal(err)
			}

			b := tx.Bucket([]byte(statusTblName))
			if b != nil {
				if err := b.Put([]byte(id), []byte(status)); err != nil {
					tx.Rollback()
					log.Fatal(err)
				} else {
					go func() {
						body := api.UpdateToRemote(id, status)
						log.Printf("device %s update remote status to %s, %s", id, status, body)
					}()
				}
			}

			if err := tx.Commit(); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}
		}
		if twinName == "gateway" {
			tx, err := boltDB.Begin(true)
			if err != nil {
				log.Fatal(err)
			}

			b := tx.Bucket([]byte(gatewayTblName))
			if b != nil {
				if err := b.Put([]byte(id), []byte(twinValue)); err != nil {
					tx.Rollback()
					log.Fatal(err)
				}
				log.Printf("%s's gateway: %s", id, twinValue)
			}

			if err := tx.Commit(); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}
		}

		t := common.MatchType(twinValue)
		if t == "float" {
			upstream[twinName] = strings.TrimRight(twinValue, "f")
			val, err := strconv.ParseFloat(twinValue, 64)
			if err == nil {
				downstream[twinName] = int(val * 100)
			}
		} else if t == "int" {
			upstream[twinName] = twinValue
			val, err := strconv.Atoi(twinValue)
			if err == nil {
				downstream[twinName] = val
			}
		} else {
			upstream[twinName] = twinValue
			downstream[twinName] = twinValue
		}
	}

	// publish downstream
	publishToDevice(cli, id, downstream)

	// publish upstream
	upstream["id"] = id
	publishToMqtt(cli, upstream, true)
}

// OperateMemGet function is used to process membership get message
var OperateMemGet mqtt.MessageHandler = func(cli mqtt.Client, msg mqtt.Message) {
	log.Info("Receive msg: ", string(msg.Payload()))
	current := make(map[string]string)
	if err := json.Unmarshal(msg.Payload(), &current); err != nil {
		log.Errorf("unmarshal receive msg, error %v\n", err)
		return
	}

	node := current["node"]
	if node == "" {
		return
	}

	// forward message, publish to mqtt broker
	memGet := common.MemETPrefix + node + common.MemETGetSuffix
	t := common.GetTimestamp()
	membershipMessage := common.CreateBaseMessage(t)
	membershipGetBody, _ := json.Marshal(membershipMessage)

	cli.Publish(memGet, 0, false, membershipGetBody)
}

// OperateMemGetResult function is used to process membership get result message
var OperateMemGetResult mqtt.MessageHandler = func(cli mqtt.Client, msg mqtt.Message) {
	node := common.GetNodeName(msg.Topic())
	if node == "" {
		log.Fatal("Wrong topic")
		return
	}

	log.Info("Receive msg: ", string(msg.Payload()))
	var detail common.MembershipDetail
	if err := json.Unmarshal(msg.Payload(), &detail); err != nil {
		log.Errorf("Unmarshal message failed: %v", err)
		return
	}

	topic := "membership/result"
	// return the array of devices temporarily
	membershipGetResultBody, _ := json.Marshal(detail.Devices)
	cli.Publish(topic, 0, false, membershipGetResultBody)
}

func handleZigbee(cli mqtt.Client) {
	topic := "zigbee/+"
	cli.Subscribe(topic, 0, OperateUpdateUpstream)
}

func handleWifi(cli mqtt.Client) {
	topic := "wifi/+"
	cli.Subscribe(topic, 0, OperateUpdateUpstream)
}

func handleBluetooth(cli mqtt.Client) {
	topic := "bluetooth/+"
	cli.Subscribe(topic, 0, OperateUpdateUpstream)
}

func handleNBIoT(cli mqtt.Client) {
	topic := "nbiot/+"
	cli.Subscribe(topic, 0, OperateUpdateUpstream)
}

func handleDownstream(cli mqtt.Client) {
	topic := common.DeviceETPrefix + "+" + common.TwinETDeltaSuffix
	cli.Subscribe(topic, 0, OperateUpdateDownstream)
}

func handleMembership(cli mqtt.Client) {
	topic := "membership"
	cli.Subscribe(topic, 0, OperateMemGet)
}

func handleMembershipResult(cli mqtt.Client) {
	topic := common.MemETPrefix + "+" + common.MemETGetResultSuffix
	cli.Subscribe(topic, 0, OperateMemGetResult)
}

func publishToMqtt(cli mqtt.Client, current map[string]interface{}, ack bool) {
	id := current["id"].(string)
	if id == "" {
		return
	}

	// check status
	c.Set(id, &cli, cache.DefaultExpiration)

	// if disabled, do not send msg
	if !ack && isDisabled(id) {
		return
	}

	// update online status
	if !ack {
		updateToOnlineStatus(cli, id)
	}

	// forward message
	deviceTwinUpdate := common.DeviceETPrefix + id + common.TwinETUpdateSuffix
	t := common.GetTimestamp()
	for f, v := range current {
		if f == "id" {
			continue
		}
		go func(field string, value interface{}, timestamp int64) {
			updateMessage := common.CreateActualUpdateMessage(field, common.Itos(value), timestamp)
			twinUpdateBody, _ := json.Marshal(updateMessage)

			cli.Publish(deviceTwinUpdate, 0, false, twinUpdateBody)
		}(f, v, t)
	}
}

func publishToDevice(cli mqtt.Client, id string, current map[string]interface{}) {
	// forward message
	tx, err := boltDB.Begin(true)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	b := tx.Bucket([]byte(gatewayTblName))
	if b != nil {
		val := b.Get([]byte(id))
		gateway := string(val)
		log.Printf("%s's gateway: %s", id, gateway)
		if gateway != "" {
			deviceTwinUpdateDelta := gateway
			for f, v := range current {
				if f == "gateway" {
					continue
				}

				go func(field string, value interface{}) {
					msg := fmt.Sprintf("{\"id\":\"%s\",\"cmd\":1,\"%s\":%s}", id, field, common.Itos(value))
					cli.Publish(deviceTwinUpdateDelta, 0, false, []byte(msg))
				}(f, v)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
}

func isDisabled(deviceID string) bool {
	tx, err := boltDB.Begin(true)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	ret := false
	b := tx.Bucket([]byte(statusTblName))
	if b != nil {
		val := b.Get([]byte(deviceID))
		status := string(val)
		if status == disabled {
			ret = true
		}
	}

	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
	return ret
}

func updateToOnlineStatus(cli mqtt.Client, deviceID string) {
	tx, err := boltDB.Begin(true)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	b := tx.Bucket([]byte(statusTblName))
	if b != nil {
		val := b.Get([]byte(deviceID))
		status := string(val)
		if status != online {
			deviceTwinUpdate := common.DeviceETPrefix + deviceID + common.TwinETUpdateSuffix
			updateMessage := common.CreateActualUpdateMessage("status", online, common.GetTimestamp())
			twinUpdateBody, _ := json.Marshal(updateMessage)
			log.Printf("device %s is online", deviceID)

			cli.Publish(deviceTwinUpdate, 0, false, twinUpdateBody)
			if err = b.Put([]byte(deviceID), []byte(online)); err != nil {
				log.Fatal(err)
			} else {
				go func() {
					body := api.UpdateToRemote(deviceID, online)
					log.Printf("device %s update remote status to %s, %s", deviceID, online, body)
				}()
			}
		}
	}

	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
}

func updateToOfflineStatus(deviceID string, v interface{}) {
	tx, err := boltDB.Begin(true)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	b := tx.Bucket([]byte(statusTblName))
	if b != nil {
		cli := *v.(*mqtt.Client)
		deviceTwinUpdate := common.DeviceETPrefix + deviceID + common.TwinETUpdateSuffix
		updateMessage := common.CreateActualUpdateMessage("status", offline, common.GetTimestamp())
		twinUpdateBody, _ := json.Marshal(updateMessage)
		log.Printf("device %s is offline", deviceID)

		cli.Publish(deviceTwinUpdate, 0, false, twinUpdateBody)
		if err = b.Put([]byte(deviceID), []byte(offline)); err != nil {
			log.Fatal(err)
		} else {
			go func() {
				body := api.UpdateToRemote(deviceID, offline)
				log.Printf("device %s update remote status to %s, %s", deviceID, offline, body)
			}()
		}
	}

	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
}
