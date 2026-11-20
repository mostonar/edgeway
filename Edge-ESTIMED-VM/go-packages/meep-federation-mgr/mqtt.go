/*
 * Copyright (c) 2024  The AdvantEDGE Authors
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

package federationmgr

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/google/uuid"
)

const (
	RetainMessageFlag = true
)

type message_broker_mqtt struct {
	running     bool
	opts        *mqtt.ClientOptions
	mqtt_notify func(msg string, systemId string)
}

var client mqtt.Client
var _fed_notify func(msg string, systemId string)

// var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
// 	log.Info("Received message: ", msg.Payload(), "on topic ", msg.Topic())
// 	fmt.Println("Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
// }
// func onMessageReceived(client mqtt.Client, msg mqtt.Message) {
// 	log.Info(">>> onMessageReceived: ", string(msg.Payload()), " on topic ", msg.Topic())
// 	go func() {
// 		log.Info("onMessageReceived: Received message: ", string(msg.Payload()), " on topic ", msg.Topic())
// 		s := strings.Split(msg.Topic(), "/")
// 		log.Info("onMessageReceived: s: ", s)
// 		_fed_notify(string(msg.Payload()), s[len(s)-1])
// 	}()
// }

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	log.Info("onMessageReceived: Received message: ", string(msg.Payload()), " on topic ", msg.Topic())
	s := strings.Split(msg.Topic(), "/")
	log.Info("onMessageReceived: s: ", s)
	_fed_notify(string(msg.Payload()), s[len(s)-1])
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	go func() {
		log.Info("Connected")
		fmt.Println("====> Connected")
	}()
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Info("Connect lost:", err)
}

func (broker_mqtt *message_broker_mqtt) Init(tm *FederationMgr) (err error) {
	log.Debug(">>> message_broker_mqtt: Init")
	log.Debug("Add brocker: tm.broker:", tm.broker)

	broker_mqtt.running = false

	broker_mqtt.opts = mqtt.NewClientOptions()
	broker_mqtt.opts.SetDefaultPublishHandler(messagePubHandler /*onMessageReceived*/)
	broker_mqtt.opts.SetClientID("meep-federation-" + uuid.New().String())
	broker_mqtt.opts.OnConnect = connectHandler
	broker_mqtt.opts.OnConnectionLost = connectLostHandler
	broker_mqtt.opts.SetUsername("")
	broker_mqtt.opts.SetPassword("")
	if strings.Index(tm.broker, "mqtts://") == 0 || strings.Index(tm.broker, "wss://") == 0 {
		broker_mqtt.opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}
	broker_mqtt.opts.AddBroker(tm.broker)
	client = mqtt.NewClient(broker_mqtt.opts)

	log.Info("Connect to MQTT server...")
	token := client.Connect()
	if token.Error() != nil {
		log.Error(token.Error())
		return token.Error()
	}
	token.Wait()

	// Subscribe
	topic := tm.topic + "/+"
	log.Info("Subscribe to: ", topic)       // Use wild card to get all MEC Sandox registered
	token = client.Subscribe(topic, 0, nil) // qos:0
	if token.Error() != nil {
		log.Error(token.Error())
		return token.Error()
	}
	token.Wait()

	_fed_notify = broker_mqtt.mqtt_notify
	broker_mqtt.running = true
	log.Info("mqtt.Init: Client is connected")

	return nil
}

func (broker_mqtt *message_broker_mqtt) Run(tm *FederationMgr) (err error) {
	log.Debug(">>> message_broker_mqtt: Run")

	// Sanity checks
	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	return nil
}

func (broker_mqtt *message_broker_mqtt) Stop(tm *FederationMgr) (err error) {
	log.Debug(">>> message_broker_mqtt: Stop")

	// Sanity checks
	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	token := client.Unsubscribe(tm.topic)
	if token.Error() != nil {
		log.Error(token.Error())
		// Continue
	}
	token.Wait()
	client.Disconnect(250)
	broker_mqtt.running = false

	return nil
}

func (broker_mqtt *message_broker_mqtt) Send(tm *FederationMgr, msgContent string, systemId string) (err error) {
	log.Info("message_broker_mqtt: Send")

	// Sanity checks
	if !broker_mqtt.running {
		err := errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	// Publish message
	log.Info("message_broker_simu: Send: Publish content : ", msgContent)
	token := client.Publish(tm.topic+"/"+systemId, 0, RetainMessageFlag, msgContent)
	token.Wait()

	return nil
}
