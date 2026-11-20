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
	"errors"
	"net/url"
	"sync"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

// FED Manager
type FederationMgr struct {
	name           string
	namespace      string
	broker         string
	topic          string
	fed_notify     func(string, string)
	connected      bool
	mutex          sync.Mutex
	message_broker message_broker_interface
}

// Enable profiling
const profiling = false

var profilingTimers map[string]time.Time

var brokerRunning bool = false

// Profiling init
func init() {
	if profiling {
		profilingTimers = make(map[string]time.Time)
	}
}

// NewFederationMgr - Creates and initializes a new FED Federation Manager
func NewFederationMgr(name string, namespace string, broker string, topic string, fed_notify func(msg string, systemId string)) (fm *FederationMgr, err error) {
	if name == "" {
		err = errors.New("Missing connector name")
		return nil, err
	}

	// Create new Federation Manager
	fm = new(FederationMgr)
	fm.name = name
	if namespace != "" {
		fm.namespace = namespace
	} else {
		fm.namespace = "default"
	}
	fm.broker = broker
	fm.topic = topic
	fm.fed_notify = fed_notify

	fm.connected = true
	return fm, nil
}

// DeleteFederationMgr -
func (fm *FederationMgr) DeleteFederationMgr(systemId string) (err error) {
	log.Info("DeleteFederationMgr: systemId: ", systemId)

	if systemId != "" {
		err := fm.PublishMessageOnMessageBroker("", systemId) // Empty message to remove retain message
		if err != nil {
			log.Error(err.Error())
			return err
		}
	}

	return nil
}

/*
 * PublishMessageOnMessageBroker publish the provided FED SystemInfo messages on brocker server (e.g. MQTT)
 * @param {string} msgContent The message to publish
 * @return {struct} nil on success, error otherwise
 */
func (fm *FederationMgr) PublishMessageOnMessageBroker(msgContent string, systemId string) (err error) {
	log.Info("PublishMessageOnMessageBroker: brokerRunning: brokerRunning=", brokerRunning)
	log.Info("PublishMessageOnMessageBroker: brokerRunning: msgContent=", msgContent)

	if !brokerRunning {
		err = errors.New("Message broker mechanism not initialized")
		log.Error(err.Error())
		return err
	}
	return fm.message_broker.Send(fm, msgContent, systemId)
}

/*
 * StartFedMessageBrokerServer start the brocker server (e.g. MQTT)
 * @return {struct} nil on success, error otherwise
 */
func (fm *FederationMgr) StartFedMessageBrokerServer() (err error) {
	log.Info("StartFedMessageBrokerServer: fm.broker: ", fm.broker)
	log.Info("StartFedMessageBrokerServer: fm.topic: ", fm.topic)

	u, err := url.ParseRequestURI(fm.broker)
	if err != nil {
		err = errors.New("Failed to parse url " + fm.broker)
		log.Error(err.Error())
		return err
	}
	//log.Info("url:%v - scheme:%v - host:%v - Path:%v - Port:%s", u, u.Scheme, u.Hostname(), u.Path, u.Port())
	if u.Scheme == "mqtt" || u.Scheme == "ws" || u.Scheme == "wss" {
		fm.message_broker = &message_broker_mqtt{false, nil, fm.fed_notify}
	} else {
		err = errors.New("Invalid url " + fm.broker)
		log.Error(err.Error())
		return err
	}

	err = fm.message_broker.Init(fm)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	err = fm.message_broker.Run(fm)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	brokerRunning = true
	log.Info("StartFedMessageBrokerServer: brokerRunning: ", brokerRunning)

	return nil
}

/*
 * StopFedMessageBrokerServer shutdown the brocker server (e.g. MQTT)
 * @return {struct} nil on success, error otherwise
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Annex D (informative): State-of-the-art of using a Message Broker as FED Message Distribution Server for exchanging non-session based FED messages
 */
func (fm *FederationMgr) StopFedMessageBrokerServer() {
	log.Info("StopFedMessageBrokerServer: brokerRunning: ", brokerRunning)

	if brokerRunning {
		brokerRunning = false
		if fm.message_broker != nil {
			_ = fm.message_broker.Stop(fm)
		}
	}
}
