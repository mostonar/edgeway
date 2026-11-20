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
	//"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	//"errors"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	amqp "github.com/streadway/amqp"
)

type message_broker_amqp struct {
	running bool
	client  *amqp.Connection
	channel *amqp.Channel
	queue_  amqp.Queue
}

func (broker_amqp *message_broker_amqp) Init(tm *TrafficMgr) (err error) {
	log.Debug(">>> message_broker_amqp: Init")

	broker_amqp.running = false

	u, err := url.ParseRequestURI(tm.broker)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	log.Info("url: ", u, u.Scheme)

	log.Info("AMQP server: ", fmt.Sprintf("amqp://%s:%s", u.Hostname(), u.Port()))
	broker_amqp.client, err = amqp.Dial(fmt.Sprintf("amqp://%s:%s", u.Hostname(), u.Port()))
	if err != nil {
		log.Error(err.Error())
		return err
	}
	broker_amqp.channel, err = broker_amqp.client.Channel()
	if err != nil {
		log.Error(err.Error())
		broker_amqp.client.Close()
		return err
	}

	err = broker_amqp.channel.QueueBind(tm.topic, "", "ETSI", false, nil)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	broker_amqp.running = true
	log.Info("amqp.Init: Connected to the server")

	return nil
}

func (broker_amqp *message_broker_amqp) Run(tm *TrafficMgr) (err error) {
	log.Debug(">>> message_broker_amqp: Run")

	// Sanity checks
	if !broker_amqp.running {
		err = errors.New("MQTT not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	return nil
}

func (broker_amqp *message_broker_amqp) Stop(tm *TrafficMgr) (err error) {
	log.Debug(">>> message_broker_amqp: Stop")

	// Sanity checks
	if !broker_amqp.running {
		err = errors.New("AMQP not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	broker_amqp.channel.Close()
	broker_amqp.client.Close()
	broker_amqp.running = false

	return nil
}

func (broker_amqp *message_broker_amqp) Send(tm *TrafficMgr, msgContent string, msgEncodeFormat string, stdOrganization string, msgType *int32) (err error) {
	log.Debug(">>> message_broker_amqp: Send")

	// Sanity checks
	if !broker_amqp.running {
		err := errors.New("AMQP not initialized or diconnected")
		log.Error(err.Error())
		return err
	}

	return nil
}
