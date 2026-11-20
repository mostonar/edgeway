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
 * distributed under the License is distributed on ance "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package federationmgr

import (
	"fmt"
	"testing"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

const ( // FIXME To be update with correct values at the end
	fmName        = "pc"
	fmNamespace   = "postgis-ns"
	v2xBrokerTest = "mqtt://172.29.10.56:1883"
	v2xTopicTest  = "ETSI/MEC/Federation"
	systemInfo1   = "{\"SystemInfo\": {\"systemId\": \"systemId1\",\"systemName\": \"systemName1\",\"systemProvider\": \"systemProvider1\"}}"
)

func fed_notify(msg string, systemId string) {
	fmt.Println(">>> fed_notify: msg: ", msg)
	fmt.Println(">>> fed_notify: systemId: ", systemId)
}

func TestNewFederationMgr(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Invalid Connector
	fmt.Println("Invalid FED Asset Manager")
	fm, err := NewFederationMgr("", fmNamespace, v2xBrokerTest, v2xTopicTest, fed_notify)
	if err == nil || fm != nil {
		t.Fatalf("DB connection should have failed")
	}

	// Valid Connector
	fmt.Println("Create valid FED Asset Manager")
	fm, err = NewFederationMgr(fmName, fmNamespace, v2xBrokerTest, v2xTopicTest, fed_notify)
	if err != nil {
		t.Fatalf("Failed to create FED Asset Manager")
	}

	fm.DeleteFederationMgr("")

	// t.Fatalf("DONE")
}

func TestDeleteFederationMgr(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	fmt.Println("Create valid FED Asset Manager")
	fm, err := NewFederationMgr(fmName, fmNamespace, v2xBrokerTest, v2xTopicTest, fed_notify)
	if err != nil || fm == nil {
		t.Fatalf("Failed to create FED Asset Manager")
	}

	err = fm.DeleteFederationMgr("")
	if err != nil {
		t.Fatalf("Failed to delete FED Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestStartFedMessageBrokerServer(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	fmt.Println("Create valid FED Asset Manager")
	fm, err := NewFederationMgr(fmName, fmNamespace, v2xBrokerTest, v2xTopicTest, fed_notify)
	if err != nil || fm == nil {
		t.Fatalf("Failed to create FED Asset Manager")
	}

	err = fm.StartFedMessageBrokerServer()
	if err != nil {
		t.Fatalf("Failed to start message brocker")
	}

	fm.StopFedMessageBrokerServer()

	err = fm.DeleteFederationMgr()
	if err != nil {
		t.Fatalf("Failed to delete FED Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestPublishMessageOnMessageBroker(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	fmt.Println("Create valid FED Asset Manager")
	fm, err := NewFederationMgr(fmName, fmNamespace, v2xBrokerTest, v2xTopicTest, fed_notify)
	if err != nil || fm == nil {
		t.Fatalf("Failed to create FED Asset Manager")
	}

	err = fm.StartFedMessageBrokerServer()
	if err != nil {
		t.Fatalf("Failed to start message brocker")
	}

	err = fm.PublishMessageOnMessageBroker(systemInfo1)
	if err != nil {
		t.Fatalf("Failed to publish a message")
	}

	fm.StopFedMessageBrokerServer()

	err = fm.DeleteFederationMgr()
	if err != nil {
		t.Fatalf("Failed to delete FED Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestPublishMessageOnMessageBrokerFailure(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	fmt.Println("Create valid FED Asset Manager")
	fm, err := NewFederationMgr(fmName, fmNamespace, v2xBrokerTest, v2xTopicTest, fed_notify)
	if err != nil || fm == nil {
		t.Fatalf("Failed to create FED Asset Manager")
	}

	// Server not started
	err = fm.PublishMessageOnMessageBroker(systemInfo1)
	if err == nil {
		t.Fatalf("Publishing a message must fail")
	}

	err = fm.DeleteFederationMgr()
	if err != nil {
		t.Fatalf("Failed to delete FED Asset Manager")
	}

	// t.Fatalf("DONE")
}
