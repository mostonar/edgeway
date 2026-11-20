/*
 * Copyright (c) 2022  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vistrafficmgr

import (
	"crypto/tls"
	"encoding/hex"
	"errors"
	"strings"
	"strconv"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type message_broker_mqtt struct {
	running    bool
	opts       *mqtt.ClientOptions
	client     mqtt.Client
	v2x_notify func(v2xMessage []byte, v2xType int32, msgProtocolVersion int32, stdOrganization string, longitude *float32, latitude *float32)
}

var _v2x_notify func(v2xMessage []byte, v2xType int32, msgProtocolVersion int32, stdOrganization string, longitude *float32, latitude *float32)
var _tm_topic string

var msgTypeMap = map[string]int32{
	"denm":              1,
	"cam":               2,
	"poi":               3,
	"spatem":            4,
	"mapem":             5,
	"ivim":              6,
	"ev_rsr":            7,
	"tistpgtransaction": 8,
	"srem":              9,
	"ssem":              10,
	"evcsn":             11,
	"saem":              12,
	"rtcmem":            13,
	"cpm":               14,
	"imzm":              15,
	"vam":               16,
	"dsm":               17,
	"mim":               18,
	"mvm":               19,
	"mcm":               20,
}

func onMessageReceived(client mqtt.Client, msg mqtt.Message) {
	go func() {
		log.Info("onMessageReceived: Received message: ", msg.Payload(), " on topic ", msg.Topic())
		if _v2x_notify != nil {
			var stdOrg string = "ETSI" // default
			var msgType int32 = 16       // default VAM
			var version int32 = 2        // default

			topicSuffix := strings.TrimPrefix(msg.Topic(), _tm_topic+"/")
			parts := strings.Split(topicSuffix, "/")

			if len(parts) >= 1 {
				mTypeStr := parts[0]
				if len(parts) == 3 {
					stdOrg = parts[0]
					mTypeStr = parts[1]
					if v, err := strconv.ParseInt(parts[2], 10, 32); err == nil {
						version = int32(v)
					}
				} else if len(parts) == 2 {
					mTypeStr = parts[0]
					if v, err := strconv.ParseInt(parts[1], 10, 32); err == nil {
						version = int32(v)
					}
				}
				
				if val, ok := msgTypeMap[strings.ToLower(mTypeStr)]; ok {
					msgType = val
				}
			}

			_v2x_notify(msg.Payload(), msgType, version, stdOrg, nil, nil)
		} else {
			log.Info("onMessageReceived: null pointer for the callback")
		}
	}()
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	go func() {
		log.Info("mqtt.OnConnectHandler: Connected")
	}()
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Info("Connect lost:", err)
}

func (broker_mqtt *message_broker_mqtt) Init(tm *TrafficMgr) (err error) {
	log.Debug(">>> Init: ", tm.broker)

	broker_mqtt.running = false

	broker_mqtt.opts = mqtt.NewClientOptions()
	broker_mqtt.opts.SetDefaultPublishHandler(onMessageReceived)
	broker_mqtt.opts.SetClientID("AdvantEDGE.meep-vis-traffic-mgr")
	broker_mqtt.opts.OnConnect = connectHandler
	broker_mqtt.opts.OnConnectionLost = connectLostHandler
	broker_mqtt.opts.CleanSession = true
	broker_mqtt.opts.SetUsername("")
	broker_mqtt.opts.SetPassword("")
	if strings.Index(tm.broker, "mqtts://") == 0 || strings.Index(tm.broker, "wss://") == 0 {
		broker_mqtt.opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}
	broker_mqtt.opts.AddBroker(tm.broker)
	broker_mqtt.client = mqtt.NewClient(broker_mqtt.opts)

	log.Info("Init: Connect to MQTT server...")
	token := broker_mqtt.client.Connect()
	if token.Error() != nil {
		log.Error(token.Error())
		return token.Error()
	}
	token.Wait()

	// Subscribe
	log.Info("Init: Subscribe to: ", tm.topic+"/#")
	token = broker_mqtt.client.Subscribe(tm.topic+"/#", 0, nil) // qos:0
	if token.Error() != nil {
		log.Error(token.Error())
		return token.Error()
	}
	token.Wait()

	_tm_topic = tm.topic
	_v2x_notify = broker_mqtt.v2x_notify
	if _v2x_notify == nil {
		log.Error("Init: _v2x_notify is nil")
	}

	broker_mqtt.running = true
	log.Info("Init: Client is connected")

	return nil
}

func (broker_mqtt *message_broker_mqtt) Run(tm *TrafficMgr) (err error) {
	log.Debug(">>> message_broker_mqtt: Run")

	// Sanity checks
	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	return nil
}

func (broker_mqtt *message_broker_mqtt) Stop(tm *TrafficMgr) (err error) {
	log.Debug(">>> message_broker_mqtt: Stop")

	// Sanity checks
	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	token := broker_mqtt.client.Unsubscribe(tm.topic)
	if token.Error() != nil {
		log.Error(token.Error())
		// Continue
	}
	token.Wait()
	broker_mqtt.client.Disconnect(250)
	broker_mqtt.running = false

	return nil
}

func (broker_mqtt *message_broker_mqtt) Send(tm *TrafficMgr, msgContent string, msgEncodeFormat string, stdOrganization string, msgType *int32) (err error) {
	log.Info("message_broker_mqtt: Send")

	// Sanity checks
	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	// Publish message
	if msgEncodeFormat == "hexadump" {
		content, err := hex.DecodeString(msgContent)
		if err != nil {
			log.Error(err.Error())
			return err
		}
		log.Info("message_broker_mqtt: Send: Publish content : ", content)
		log.Info("message_broker_mqtt: Send: msgEncodeFormat: ", msgEncodeFormat)
		log.Info("message_broker_mqtt: Send: stdOrganization: ", stdOrganization)
		token := broker_mqtt.client.Publish(tm.topic, 0, false, content)
		token.Wait()

		return nil
	}

	err = errors.New("MQTT encoding not supported, message is discarded")
	log.Error(err.Error())
	return err
}
