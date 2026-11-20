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

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	//	met "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics"
	mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"

	"github.com/gorilla/mux"
)

//json format using spacing to facilitate reading
const testScenario string = `
 {
	 "version":"1.5.3",
	 "name":"test-scenario",
	 "deployment":{
		 "netChar":{
			 "latency":50,
			 "latencyVariation":5,
			 "throughputDl":1000,
			 "throughputUl":1000
		 },
		 "domains":[
			 {
				 "id":"PUBLIC",
				 "name":"PUBLIC",
				 "type":"PUBLIC",
				 "netChar":{
					 "latency":6,
					 "latencyVariation":2,
					 "throughputDl":1000000,
					 "throughputUl":1000000
				 },
				 "zones":[
					 {
						 "id":"PUBLIC-COMMON",
						 "name":"PUBLIC-COMMON",
						 "type":"COMMON",
						 "netChar":{
							 "latency":5,
							 "latencyVariation":1,
							 "throughput":1000000
						 },
						 "networkLocations":[
							 {
								 "id":"PUBLIC-COMMON-DEFAULT",
								 "name":"PUBLIC-COMMON-DEFAULT",
								 "type":"DEFAULT",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":50000,
									 "throughputUl":50000,
									 "packetLoss":1
								 }
							 }
						 ]
					 }
				 ]
			 },
			 {
				 "id":"4da82f2d-1f44-4945-8fe7-00c0431ef8c7",
				 "name":"operator-cell1",
				 "type":"OPERATOR-CELLULAR",
				 "netChar":{
					 "latency":6,
					 "latencyVariation":2,
					 "throughputDl":1000,
					 "throughputUl":1000
				 },
				 "cellularDomainConfig":{
					 "mnc":"456",
					 "mcc":"123",
					 "defaultCellId":"1234567"
				 },
				 "zones":[
					 {
						 "id":"operator-cell1-COMMON",
						 "name":"operator-cell1-COMMON",
						 "type":"COMMON",
						 "netChar":{
							 "latency":5,
							 "latencyVariation":1,
							 "throughput":1000
						 },
						 "networkLocations":[
							 {
								 "id":"operator-cell1-COMMON-DEFAULT",
								 "name":"operator-cell1-COMMON-DEFAULT",
								 "type":"DEFAULT",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 }
							 }
						 ]
					 },
					 {
						 "id":"0836975f-a7ea-41ec-b0e0-aff43178194d",
						 "name":"zone1",
						 "type":"ZONE",
						 "netChar":{
							 "latency":5,
							 "latencyVariation":1,
							 "throughput":1000
						 },
						 "networkLocations":[
							 {
								 "id":"zone1-DEFAULT",
								 "name":"zone1-DEFAULT",
								 "type":"DEFAULT",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 },
								 "physicalLocations":[
									 {
										 "id":"97b80da7-a74a-4649-bb61-f7fa4fbb2d76",
										 "name":"zone1-edge1",
										 "type":"EDGE",
										 "connected":true,
										 "processes":[
											 {
												 "id":"fcf1269c-a061-448e-aa80-6dd9c2d4c548",
												 "name":"zone1-edge1-iperf",
												 "type":"EDGE-APP",
												 "image":"meep-docker-registry:30001/iperf-server",
												 "commandArguments":"-c, export; iperf -s -p $IPERF_SERVICE_PORT",
												 "commandExe":"/bin/bash",
												 "serviceConfig":{
													 "name":"zone1-edge1-iperf",
													 "meSvcName":"iperf",
													 "ports":[
														 {
															 "protocol":"UDP",
															 "port":80
														 }
													 ]
												 },
												 "netChar":{
													 "throughputDl":1000,
													 "throughputUl":1000
												 }
											 },
											 {
												 "id":"35697e68-c627-4b8d-9cd7-ad8b8e226aee",
												 "name":"zone1-edge1-svc",
												 "type":"EDGE-APP",
												 "image":"meep-docker-registry:30001/demo-server",
												 "environment":"MGM_GROUP_NAME=svc, MGM_APP_ID=zone1-edge1-svc, MGM_APP_PORT=80",
												 "serviceConfig":{
													 "name":"zone1-edge1-svc",
													 "meSvcName":"svc",
													 "ports":[
														 {
															 "protocol":"TCP",
															 "port":80
														 }
													 ]
												 },
												 "netChar":{
													 "throughputDl":1000,
													 "throughputUl":1000
												 }
											 }
										 ],
										 "netChar":{
											 "throughputDl":1000,
											 "throughputUl":1000
										 }
									 }
								 ]
							 },
							 {
								 "id":"7a6f8077-b0b3-403d-b954-3351e21afeb7",
								 "name":"zone1-poa-cell1",
								 "type":"POA-4G",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 },
								 "poa4GConfig":{
									 "cellId":"2345678"
								 },
								 "geoData": {
									 "location": {
										 "type": "Point",
											 "coordinates": [
												 7.423547,
												 43.731724
									 ]
									 },
									 "radius": 400,
									 "path": null,
									 "eopMode": null,
									 "velocity": null
								 },
								 "physicalLocations":[
									 {
										 "id":"32a2ced4-a262-49a8-8503-8489a94386a2",
										 "name":"ue1",
										 "type":"UE",
										 "connected":true,
										 "wireless":true,
										 "processes":[
											 {
												 "id":"9bdd6acd-f6e4-44f6-a26c-8fd9abd338a7",
												 "name":"ue1-iperf",
												 "type":"UE-APP",
												 "image":"meep-docker-registry:30001/iperf-client",
												 "commandArguments":"-c, export; iperf -u -c $IPERF_SERVICE_HOST -p $IPERF_SERVICE_PORT\n-t 3600 -b 50M;",
												 "commandExe":"/bin/bash",
												 "netChar":{
													 "throughputDl":1000,
													 "throughputUl":1000
												 }
											 }
										 ],
										 "netChar":{
											 "throughputDl":1000,
											 "throughputUl":1000
										 }
									 },
									 {
										 "id":"b1851da5-c9e1-4bd8-ad23-5925c82ee127",
										 "name":"zone1-fog1",
										 "type":"FOG",
										 "connected":true,
										 "processes":[
											 {
												 "id":"c2f2fb5d-4053-4cee-a0ee-e62bbb7751b6",
												 "name":"zone1-fog1-iperf",
												 "type":"EDGE-APP",
												 "image":"meep-docker-registry:30001/iperf-server",
												 "commandArguments":"-c, export; iperf -s -p $IPERF_SERVICE_PORT;",
												 "commandExe":"/bin/bash",
												 "serviceConfig":{
													 "name":"zone1-fog1-iperf",
													 "meSvcName":"iperf",
													 "ports":[
														 {
															 "protocol":"UDP",
															 "port":80
														 }
													 ]
												 },
												 "netChar":{
													 "throughputDl":1000,
													 "throughputUl":1000
												 }
											 },
											 {
												 "id":"53b5806b-e213-4c5a-a181-f1c31c24287b",
												 "name":"zone1-fog1-svc",
												 "type":"EDGE-APP",
												 "image":"meep-docker-registry:30001/demo-server",
												 "environment":"MGM_GROUP_NAME=svc, MGM_APP_ID=zone1-fog1-svc, MGM_APP_PORT=80",
												 "serviceConfig":{
													 "name":"zone1-fog1-svc",
													 "meSvcName":"svc",
													 "ports":[
														 {
															 "protocol":"TCP",
															 "port":80
														 }
													 ]
												 },
												 "netChar":{
													 "throughputDl":1000,
													 "throughputUl":1000
												 }
											 }
										 ],
										 "netChar":{
											 "throughputDl":1000,
											 "throughputUl":1000
										 }
									 }
								 ]
							 },
							 {
								 "id":"7ff90180-2c1a-4c11-b59a-3608c5d8d874",
								 "name":"zone1-poa-cell2",
								 "type":"POA-4G",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 },
								 "poa4GConfig":{
									 "cellId":"3456789"
								 },
								 "geoData": {
									 "location": {
									 "type": "Point",
									 "coordinates": [
										 7.423547,
										 43.731724
									 ]
									 },
									 "radius": 400,
									 "path": null,
									 "eopMode": null,
									 "velocity": null
								 }
							 }
						 ]
					 },
					 {
						 "id":"d1f06b00-4454-4d35-94a5-b573888e7ea9",
						 "name":"zone2",
						 "type":"ZONE",
						 "netChar":{
							 "latency":5,
							 "latencyVariation":1,
							 "throughput":1000
						 },
						 "networkLocations":[
							 {
								 "id":"zone2-DEFAULT",
								 "name":"zone2-DEFAULT",
								 "type":"DEFAULT",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 },
								 "physicalLocations":[
									 {
										 "id":"fb130d18-fd81-43e0-900c-c584e7190302",
										 "name":"zone2-edge1",
										 "type":"EDGE",
										 "connected":true,
										 "processes":[
											 {
												 "id":"5c8276ba-0b78-429d-a0bf-d96f35ba2c77",
												 "name":"zone2-edge1-iperf",
												 "type":"EDGE-APP",
												 "image":"meep-docker-registry:30001/iperf-server",
												 "commandArguments":"-c, export; iperf -s -p $IPERF_SERVICE_PORT;",
												 "commandExe":"/bin/bash",
												 "serviceConfig":{
													 "name":"zone2-edge1-iperf",
													 "meSvcName":"iperf",
													 "ports":[
														 {
															 "protocol":"UDP",
															 "port":80
														 }
													 ]
												 },
												 "netChar":{
													 "throughputDl":1000,
													 "throughputUl":1000
												 }
											 },
											 {
												 "id":"53fa28f0-80e2-414c-8841-86db9bd37d51",
												 "name":"zone2-edge1-svc",
												 "type":"EDGE-APP",
												 "image":"meep-docker-registry:30001/demo-server",
												 "environment":"MGM_GROUP_NAME=svc, MGM_APP_ID=zone2-edge1-svc, MGM_APP_PORT=80",
												 "serviceConfig":{
													 "name":"zone2-edge1-svc",
													 "meSvcName":"svc",
													 "ports":[
														 {
															 "protocol":"TCP",
															 "port":80
														 }
													 ]
												 },
												 "netChar":{
													 "throughputDl":1000,
													 "throughputUl":1000
												 }
											 }
										 ],
										 "netChar":{
											 "throughputDl":1000,
											 "throughputUl":1000
										 }
									 }
								 ]
							 },
							 {
								 "id":"c44b8937-58af-44b2-acdb-e4d1c4a1510b",
								 "name":"zone2-poa1",
								 "type":"POA",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":20,
									 "throughputUl":20
								 }
							 }
						 ]
					 }
				 ]
			 },
			 {
				 "id":"e29138fb-cf03-4372-8335-fd2665b77a11",
				 "name":"operator1",
				 "type":"OPERATOR",
				 "netChar":{
					 "latency":6,
					 "latencyVariation":2,
					 "throughputDl":1000,
					 "throughputUl":1000
				 },
				 "zones":[
					 {
						 "id":"operator1-COMMON",
						 "name":"operator1-COMMON",
						 "type":"COMMON",
						 "netChar":{
							 "latency":5,
							 "latencyVariation":1,
							 "throughputDl":1000,
							 "throughputUl":1000
						 },
						 "networkLocations":[
							 {
								 "id":"operator1-COMMON-DEFAULT",
								 "name":"operator1-COMMON-DEFAULT",
								 "type":"DEFAULT",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 }
							 }
						 ]
					 },
					 {
						 "id":"7d8bee73-6d5c-4c5a-a3a0-49ebe3cd2c71",
						 "name":"zone3",
						 "type":"ZONE",
						 "netChar":{
							 "latency":5,
							 "latencyVariation":1,
							 "throughputDl":1000,
							 "throughputUl":1000
						 },
						 "networkLocations":[
							 {
								 "id":"zone3-DEFAULT",
								 "name":"zone3-DEFAULT",
								 "type":"DEFAULT",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 }
							 },
							 {
								 "id":"ecc2a41b-7381-4108-a037-52862c520733",
								 "name":"poa1",
								 "type":"POA",
								 "netChar":{
									 "latency":1,
									 "latencyVariation":1,
									 "throughputDl":1000,
									 "throughputUl":1000
								 }
							 }
						 ]
					 }
				 ]
			 }
		 ]
	 }
 }
 `

const redisTestAddr = "localhost:30380"
const influxTestAddr = "http://localhost:30986"
const testScenarioName = "testScenario"
const fedBrokerTest = "mqtt://172.29.10.56:1883"
const fedTopicTest = "ETSI/MEC/Federation"

var m *mod.Model
var mqLocal *mq.MsgQueue

func TestSysteminfoPost(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	/******************************
	 * expected response section
	 ******************************/
	var expected_systemInfo = SystemInfo{
		SystemId:       "",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo_str, err := json.Marshal(expected_systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_systemInfo_str: ", string(expected_systemInfo_str))

	/******************************
	 * expected request section
	 ******************************/
	var systemInfo = SystemInfo{
		SystemId:       "",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	body, err := json.Marshal(systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/fed_enablement/v1/fed_resources/systems", bytes.NewBuffer(body), nil, nil, nil, http.StatusOK, SysteminfoPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/
	fmt.Println("Respone: rr: ", rr)
	var resp SystemInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validateSystemInfo(resp, expected_systemInfo) {
		t.Errorf("handler returned unexpected body: got %v want %v", rr, expected_systemInfo_str)
	}

	testSysteminfoDelete(t, resp)

	terminateScenario()
}

func TestSysteminfoPostFail(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	/******************************
	 * expected response section
	 ******************************/
	var expected_systemInfo = SystemInfo{
		SystemId:       "",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo_str, err := json.Marshal(expected_systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_systemInfo_str: ", string(expected_systemInfo_str))

	/******************************
	 * expected request section
	 ******************************/
	var systemInfo = SystemInfo{
		SystemId:       "ShallBeEmpty",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	body, err := json.Marshal(systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/fed_enablement/v1/fed_resources/systems", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, SysteminfoPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	systemInfo = SystemInfo{
		SystemId:       "",
		SystemName:     "", // Shall not be empty
		SystemProvider: "systemProvider1",
	}
	body, err = json.Marshal(systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/fed_enablement/v1/fed_resources/systems", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, SysteminfoPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	systemInfo = SystemInfo{
		SystemId:       "",
		SystemName:     "systemName1",
		SystemProvider: "", // Shall not be empty
	}
	body, err = json.Marshal(systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/fed_enablement/v1/fed_resources/systems", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, SysteminfoPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/

	terminateScenario()
}

func testSysteminfoPost(t *testing.T, expected_systemInfo SystemInfo) (systemInfo SystemInfo) {
	fmt.Println(">>> testSysteminfoPost")

	expected_systemInfo_str, err := json.Marshal(expected_systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_systemInfo_str: ", string(expected_systemInfo_str))

	body, err := json.Marshal(expected_systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))

	rr, err := sendRequest(http.MethodPost, "/fed_enablement/v1/fed_resources/systems", bytes.NewBuffer(body), nil, nil, nil, http.StatusOK, SysteminfoPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	fmt.Println("Respone: rr: ", rr)
	err = json.Unmarshal([]byte(rr), &systemInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", systemInfo)

	return systemInfo
}

func TestSysteminfoDelete(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	var systemInfo = SystemInfo{
		SystemId:       "systemId1",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo := testSysteminfoPost(t, systemInfo)

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * expected request section
	 ******************************/

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["systemId"] = expected_systemInfo.SystemId

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodDelete, "/fed_enablement/v1/fed_resources/systems", nil, vars, nil, nil, http.StatusNoContent, SysteminfoByIdDELETE)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/
	testSysteminfoDelete(t, expected_systemInfo)

	terminateScenario()
}

func TestSysteminfoDeleteFail(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * expected request section
	 ******************************/

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["systemId"] = "Unknown"

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodDelete, "/fed_enablement/v1/fed_resources/systems", nil, vars, nil, nil, http.StatusNotFound, SysteminfoByIdDELETE)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/

	terminateScenario()
}

func testSysteminfoDelete(t *testing.T, systemInfo SystemInfo) {

	vars := make(map[string]string)
	vars["systemId"] = systemInfo.SystemId
	_, err := sendRequest(http.MethodDelete, "/fed_enablement/v1/fed_resources/systems", nil, vars, nil, nil, http.StatusNoContent, SysteminfoByIdDELETE)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")
}

func TestSysteminfoGet(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	var systemInfo = SystemInfo{
		SystemId:       "systemId1",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo := testSysteminfoPost(t, systemInfo)
	fmt.Println("testSysteminfoPost: ", expected_systemInfo)

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * expected request section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/fed_enablement/v1/fed_resources/systems", nil, nil, nil, nil, http.StatusOK, SysteminfoGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/
	fmt.Println("Respone: rr: ", rr)
	var resp []SystemInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if len(resp) == 0 {
		t.Errorf("handler returned unexpected body: empty list")
	}
	if !validateSystemInfo(resp[0], expected_systemInfo) {
		t.Errorf("handler returned unexpected body")
	}

	testSysteminfoDelete(t, expected_systemInfo)

	terminateScenario()
}

func TestSysteminfoByIdGET(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	var systemInfo1 = SystemInfo{
		SystemId:       "systemId1",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo1 := testSysteminfoPost(t, systemInfo1)
	fmt.Println("testSysteminfoPost: ", expected_systemInfo1)
	var systemInfo2 = SystemInfo{
		SystemId:       "systemId2",
		SystemName:     "systemName3",
		SystemProvider: "systemProvider3",
	}
	expected_systemInfo2 := testSysteminfoPost(t, systemInfo2)
	fmt.Println("testSysteminfoPost: ", expected_systemInfo2)

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * expected request section
	 ******************************/

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["systemId"] = expected_systemInfo1.SystemId

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/fed_enablement/v1/fed_resources/systems", nil, vars, nil, nil, http.StatusOK, SysteminfoByIdGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/
	fmt.Println("Respone: rr: ", rr)
	var resp SystemInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validateSystemInfo(resp, expected_systemInfo1) {
		t.Errorf("handler returned unexpected body")
	}

	testSysteminfoDelete(t, expected_systemInfo1)
	testSysteminfoDelete(t, expected_systemInfo2)

	terminateScenario()
}

func TestSysteminfoByIdGETFail1(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	var systemInfo1 = SystemInfo{
		SystemId:       "systemId1",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo1 := testSysteminfoPost(t, systemInfo1)
	fmt.Println("testSysteminfoPost: ", expected_systemInfo1)
	var systemInfo2 = SystemInfo{
		SystemId:       "systemId2",
		SystemName:     "systemName3",
		SystemProvider: "systemProvider3",
	}
	expected_systemInfo2 := testSysteminfoPost(t, systemInfo2)
	fmt.Println("testSysteminfoPost: ", expected_systemInfo2)

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * expected request section
	 ******************************/

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["systemId"] = "systemId3"

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodGet, "/fed_enablement/v1/fed_resources/systems", nil, vars, nil, nil, http.StatusNotFound, SysteminfoByIdGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/

	testSysteminfoDelete(t, expected_systemInfo1)
	testSysteminfoDelete(t, expected_systemInfo2)

	terminateScenario()
}

func TestSysteminfoByIdGETFail2(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * expected request section
	 ******************************/

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["systemId"] = "systemId3"

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodGet, "/fed_enablement/v1/fed_resources/systems", nil, vars, nil, nil, http.StatusNotFound, SysteminfoByIdGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/

	terminateScenario()
}

func TestServicesInfoGET(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")

	var systemInfo1 = SystemInfo{
		SystemId:       "systemId1",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo1 := testSysteminfoPost(t, systemInfo1)
	fmt.Println("testSysteminfoPost: ", expected_systemInfo1)

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * expected request section
	 ******************************/

	/******************************
	 * request vars section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/fed_enablement/v1/fed_resources/systems/"+expected_systemInfo1.SystemId+"/services", nil, nil, nil, nil, http.StatusOK, ServicesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/
	fmt.Println("Respone: rr: ", rr)
	var resp SystemInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validateSystemInfo(resp, expected_systemInfo1) {
		t.Errorf("handler returned unexpected body")
	}

	testSysteminfoDelete(t, expected_systemInfo1)

	terminateScenario()
}

func validateSystemInfo(received SystemInfo, expected SystemInfo) bool {
	fmt.Printf(">>> validateSystemInfo: received: %+v\n", received)
	fmt.Printf(">>> validateSystemInfo: expected: %+v\n", expected)

	if received.SystemId == "" {
		fmt.Println("received.SystemId mismatch")
		return false
	}
	if received.SystemName != expected.SystemName {
		fmt.Println("received.SystemName mismatch")
		return false
	}
	if received.SystemProvider != expected.SystemProvider {
		fmt.Println("received.SystemProvider mismatch")
		return false
	}

	fmt.Println("validateSystemInfo: succeed")
	return true
}

func TestServicesGet(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())
	initializeVars()
	err := Init()
	if err != nil {
		t.Fatalf("Error initializing test basic procedure")
	}
	err = Run()
	if err != nil {
		t.Fatalf("Error running test basic procedure")
	}
	fmt.Println("Set a scenario")
	initialiseScenario(testScenario)
	time.Sleep(1000 * time.Millisecond)
	updateScenario("mobility1")
	var systemInfo = SystemInfo{
		SystemId:       "",
		SystemName:     "systemName1",
		SystemProvider: "systemProvider1",
	}
	expected_systemInfo := testSysteminfoPost(t, systemInfo)
	/******************************
	 * expected response section
	 ******************************/
	/******************************
	 * expected request section
	 ******************************/
	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["systemId"] = expected_systemInfo.SystemId
	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/fed_enablement/v1/fed_resources/systems/{systemId}/services", nil, vars, nil, nil, http.StatusOK, ServicesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")
	/******************************
	 * back to initial state section
	 ******************************/
	fmt.Println("Respone: rr: ", rr)
	// var resp []SystemInfo
	// err = json.Unmarshal([]byte(rr), &resp)
	// if err != nil {
	// 	t.Fatalf(err.Error())
	// }
	// fmt.Println("Respone: resp: ", resp)
	// if len(resp) == 0 {
	// 	t.Errorf("handler returned unexpected body: empty list")
	// }
	// if !validateSystemInfo(resp[0], expected_systemInfo) {
	// 	t.Errorf("handler returned unexpected body")
	// }
	testSysteminfoDelete(t, expected_systemInfo)
	terminateScenario()
}

func initializeVars() {
	mod.DbAddress = redisTestAddr
	redisAddr = redisTestAddr
	influxAddr = influxTestAddr
	sandboxName = testScenarioName
	os.Setenv("MEEP_PREDICT_MODEL_SUPPORTED", "true")
	os.Setenv("MEEP_SANDBOX_NAME", testScenarioName)
	fed_broker = fedBrokerTest
	os.Setenv("MEEP_BROKER", fed_broker)
	os.Setenv("MEEP_TOPIC", fedTopicTest)
	os.Setenv("MEEP_PUBLIC_URL", "http://localhost")
}

func initialiseScenario(testScenario string) {

	//clear DB
	cleanUp()

	cfg := mod.ModelCfg{
		Name:      testScenarioName,
		Namespace: sandboxName,
		Module:    "test-mod",
		UpdateCb:  nil,
		DbAddr:    redisAddr,
	}
	var err error
	m, err = mod.NewModel(cfg)
	if err != nil {
		log.Error("Failed to create model: ", err)
		return
	}
	fmt.Println("initialiseScenario: model created")

	// Create message queue
	mqLocal, err = mq.NewMsgQueue(mq.GetLocalName(testScenarioName), "test-mod", testScenarioName, redisAddr)
	if err != nil {
		log.Error("Failed to create Message Queue with error: ", err)
		return
	}
	fmt.Println("Message Queue created")

	fmt.Println("Set Model")
	err = m.SetScenario([]byte(testScenario))
	if err != nil {
		log.Error("Failed to set model: ", err)
		return
	}

	err = m.Activate()
	if err != nil {
		log.Error("Failed to activate scenario with err: ", err.Error())
		return
	}

	msg := mqLocal.CreateMsg(mq.MsgScenarioActivate, mq.TargetAll, testScenarioName)
	err = mqLocal.SendMsg(msg)
	if err != nil {
		log.Error("Failed to send message: ", err)
		return
	}

	time.Sleep(100 * time.Millisecond)

}

func updateScenario(testUpdate string) {

	switch testUpdate {
	case "mobility1":
		// mobility event of ue1 to zone2-poa1
		elemName := "ue1"
		destName := "zone2-poa1"

		_, _, err := m.MoveNode(elemName, destName, nil)
		if err != nil {
			log.Error("Error sending mobility event")
		}

		msg := mqLocal.CreateMsg(mq.MsgScenarioUpdate, mq.TargetAll, testScenarioName)
		err = mqLocal.SendMsg(msg)
		if err != nil {
			log.Error("Failed to send message: ", err)
		}
	case "mobility2":
		// mobility event of ue1 to zone2-poa1
		elemName := "ue1"
		destName := "zone1-poa-cell1"

		_, _, err := m.MoveNode(elemName, destName, nil)
		if err != nil {
			log.Error("Error sending mobility event")
		}

		msg := mqLocal.CreateMsg(mq.MsgScenarioUpdate, mq.TargetAll, testScenarioName)
		err = mqLocal.SendMsg(msg)
		if err != nil {
			log.Error("Failed to send message: ", err)
		}
	case "mobility3":
		// mobility event of ue1 to zone1-poa-cell2
		elemName := "ue1"
		destName := "zone1-poa-cell2"

		_, _, err := m.MoveNode(elemName, destName, nil)
		if err != nil {
			log.Error("Error sending mobility event")
		}

		msg := mqLocal.CreateMsg(mq.MsgScenarioUpdate, mq.TargetAll, testScenarioName)
		err = mqLocal.SendMsg(msg)
		if err != nil {
			log.Error("Failed to send message: ", err)
		}
	default:
	}
	time.Sleep(100 * time.Millisecond)
}

func terminateScenario() {
	if mqLocal != nil {
		_ = Stop()
		msg := mqLocal.CreateMsg(mq.MsgScenarioTerminate, mq.TargetAll, testScenarioName)
		err := mqLocal.SendMsg(msg)
		if err != nil {
			log.Error("Failed to send message: ", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func sendRequest(method string, url string, body io.Reader, vars map[string]string, query map[string]string, location *string, code int, f http.HandlerFunc) (string, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil || req == nil {
		return "", err
	}
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	if query != nil {
		q := req.URL.Query()
		for k, v := range query {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(f)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	time.Sleep(50 * time.Millisecond)

	// Check the status code is what we expect.
	if status := rr.Code; status != code {
		s := fmt.Sprintf("Wrong status code - got %v want %v", status, code)
		return "", errors.New(s)
	}

	// Set Location header in case of POST
	if location != nil {
		s := rr.Header().Get("Location")
		if rr == nil {
			s := fmt.Sprintf("Header Location expected")
			return "", errors.New(s)
		} else if !strings.Contains(s, *location) {
			s := fmt.Sprintf("Wrong Header Location - got %s want %s", s, *location)
			return "", errors.New(s)
		}
	}

	return string(rr.Body.String()), nil
}
