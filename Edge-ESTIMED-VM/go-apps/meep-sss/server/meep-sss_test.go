/*
 * Copyright (c) 2025  The AdvantEDGE Authors
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

var m *mod.Model
var mqLocal *mq.MsgQueue

// func TestSensorDiscoveryLookupGET(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())
// 	initializeVars()
// 	err := Init()
// 	if err != nil {
// 		t.Fatalf("Error initializing test basic procedure")
// 	}
// 	err = Run()
// 	if err != nil {
// 		t.Fatalf("Error running test basic procedure")
// 	}
// 	fmt.Println("Set a scenario")
// 	initialiseScenario(testScenario)
// 	time.Sleep(1000 * time.Millisecond)
// 	updateScenario("mobility1")

// 	/******************************
// 	 * expected response section
// 	 ******************************/

// 	/******************************
// 	 * request queries section
// 	 ******************************/
// 	queries := make(map[string]string) // Mandatory filter
// 	queries["type"] = "4"

// 	/******************************
// 	 * request body section
// 	 ******************************/

// 	/******************************
// 	 * request execution section
// 	 ******************************/
// 	rr, err := sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, queries, nil, http.StatusOK, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Respone: rr: ", rr)
// 	var sensors []SensorDiscoveryInfo
// 	err = json.Unmarshal([]byte(rr), &sensors)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	if len(sensors) == 0 {
// 		t.Fatalf("SensorDiscoveryInfo list expected")
// 	}

// 	fmt.Println("Received expected response")
// 	/******************************
// 	 * back to initial state section
// 	 ******************************/
// 	terminateScenario()
// }

// func TestSensorDiscoveryLookupGET_with_opt_filter(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())
// 	initializeVars()
// 	err := Init()
// 	if err != nil {
// 		t.Fatalf("Error initializing test basic procedure")
// 	}
// 	err = Run()
// 	if err != nil {
// 		t.Fatalf("Error running test basic procedure")
// 	}
// 	fmt.Println("Set a scenario")
// 	initialiseScenario(testScenario)
// 	time.Sleep(1000 * time.Millisecond)
// 	updateScenario("mobility1")

// 	/******************************
// 	 * request queries section
// 	 ******************************/
// 	queries := make(map[string]string) // Mandatory filter
// 	queries["type"] = "4"

// 	/******************************
// 	 * request body section
// 	 ******************************/

// 	/******************************
// 	 * request execution section
// 	 ******************************/
// 	rr, err := sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, queries, nil, http.StatusOK, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Respone: rr: ", rr)
// 	var ref_sensors []SensorDiscoveryInfo
// 	err = json.Unmarshal([]byte(rr), &ref_sensors)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	if len(ref_sensors) == 0 {
// 		t.Fatalf("SensorDiscoveryInfo list expected")
// 	}
// 	fmt.Println("Received expected response")

// 	queries = make(map[string]string)
// 	queries["type"] = "4"
// 	queries["sensorPropertyList"] = "rn,cnf,con"
// 	rr, err = sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, queries, nil, http.StatusOK, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Respone: rr: ", rr)
// 	var sensors []SensorDiscoveryInfo
// 	err = json.Unmarshal([]byte(rr), &sensors)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	if len(sensors) != len(ref_sensors) {
// 		t.Fatalf("SensorDiscoveryInfo list length invalid")
// 	}
// 	fmt.Println("Received expected response")

// 	queries = make(map[string]string)
// 	queries["type"] = "4"
// 	queries["sensorPropertyList"] = "john,doe"
// 	_, err = sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, queries, nil, http.StatusNotFound, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Received expected response")

// 	queries = make(map[string]string)
// 	queries["type"] = "4"
// 	queries["sensorPropertyList"] = "rn,john,doe"
// 	_, err = sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, queries, nil, http.StatusNotFound, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Received expected response")

// 	/******************************
// 	 * back to initial state section
// 	 ******************************/
// 	terminateScenario()
// }

// func TestSensorDiscoveryLookupGET_Fail(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())
// 	initializeVars()
// 	err := Init()
// 	if err != nil {
// 		t.Fatalf("Error initializing test basic procedure")
// 	}
// 	err = Run()
// 	if err != nil {
// 		t.Fatalf("Error running test basic procedure")
// 	}
// 	fmt.Println("Set a scenario")
// 	initialiseScenario(testScenario)
// 	time.Sleep(1000 * time.Millisecond)
// 	updateScenario("mobility1")

// 	/******************************
// 	 * request queries section
// 	 ******************************/
// 	//  queries := make(map[string]string) // No queries
// 	//  queries["type"] = "4"

// 	_, err = sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, nil, nil, http.StatusBadRequest, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Received expected response")

// 	queries := make(map[string]string) // No queries
// 	queries["type"] = "5"
// 	_, err = sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, queries, nil, http.StatusNotFound, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Received expected response")

// 	/******************************
// 	 * back to initial state section
// 	 ******************************/
// 	terminateScenario()
// }

// func TestSensorStatusLookupGET(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())
// 	initializeVars()
// 	err := Init()
// 	if err != nil {
// 		t.Fatalf("Error initializing test basic procedure")
// 	}
// 	err = Run()
// 	if err != nil {
// 		t.Fatalf("Error running test basic procedure")
// 	}
// 	fmt.Println("Set a scenario")
// 	initialiseScenario(testScenario)
// 	time.Sleep(1000 * time.Millisecond)
// 	updateScenario("mobility1")

// 	queries := make(map[string]string) // Mandatory filter
// 	queries["type"] = "4"
// 	rr, err := sendRequest(http.MethodGet, "/sens/v1/queries/sensor_discovery", nil, nil, queries, nil, http.StatusOK, SensorDiscoveryLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Respone: rr: ", rr)
// 	var sensors []SensorDiscoveryInfo
// 	err = json.Unmarshal([]byte(rr), &sensors)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	if len(sensors) == 0 {
// 		t.Fatalf("SensorDiscoveryInfo list expected")
// 	}
// 	fmt.Println("Received expected response")

// 	queries = make(map[string]string)
// 	queries["sensorIdentifier"] = sensors[0].SensorIdentifier
// 	rr, err = sendRequest(http.MethodGet, "/sens/v1/queries/sensor_status", nil, nil, queries, nil, http.StatusOK, SensorStatusLookupGET)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("Respone: rr: ", rr)
// 	var status SensorStatusInfo
// 	err = json.Unmarshal([]byte(rr), &status)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	if status.SensorIdentifier != sensors[0].SensorIdentifier {
// 		t.Fatalf("SensorIdentifier mismatch")
// 	}
// 	fmt.Println("Received expected response")
// 	/******************************
// 	 * back to initial state section
// 	 ******************************/
// 	terminateScenario()
// }

func TestSensorDiscoveryEventSubscriptionPOST_no_query(t *testing.T) {
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
	subscriptionType := SENSOR_DISCOVERY_EVENT_SUBSCRIPTION
	var expected = SensorDiscoveryEventSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	jsonInfo, err := json.Marshal(&expected)
	if err != nil {
		t.Fatalf(err.Error())
	}
	expected_response := string(jsonInfo)
	fmt.Println("expected_response: ", expected_response)

	/******************************
	 * request vars section
	 ******************************/
	//vars := make(map[string]string)
	//vars["subscriptionId"] = subscription_id

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/
	var sensorDiscoveryEventSubscription = SensorDiscoveryEventSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	body, err := json.Marshal(sensorDiscoveryEventSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_discovery", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, SensorDiscoverySubscriptionPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp SensorDiscoveryEventSubscription
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !validateSensorDiscoveryEventSubscription(expected, resp) {
		t.Fatalf("handler returned unexpected body: got %v want %v", rr, expected_response)
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = deleteSensorDiscoveryEventSubscription(resp)
	terminateScenario()
}

func TestSensorDiscoveryEventSubscriptionPOST_Fail(t *testing.T) {
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
	 * request vars section
	 ******************************/
	//vars := make(map[string]string)
	//vars["subscriptionId"] = subscription_id

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/
	var sensorDiscoveryEventSubscription = SensorDiscoveryEventSubscription{
		// SubscriptionType:  &subscriptionType, // No subscription type
		CallbackReference: "http://test.org",
	}
	body, err := json.Marshal(sensorDiscoveryEventSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_discovery", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, SensorDiscoverySubscriptionPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Received expected response")

	/******************************
	 * request body section
	 ******************************/
	subscriptionType := SENSOR_DISCOVERY_EVENT_SUBSCRIPTION
	sensorDiscoveryEventSubscription = SensorDiscoveryEventSubscription{
		SubscriptionType: &subscriptionType,
		//CallbackReference: "http://test.org", // No callback
	}
	body, err = json.Marshal(sensorDiscoveryEventSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_discovery", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, SensorDiscoverySubscriptionPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Received expected response")

	/******************************
	 * request body section
	 ******************************/
	sensorDiscoveryEventSubscription = SensorDiscoveryEventSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org", // callback
		WebsockNotifConfig: &WebsockNotifConfig{ // And WebsockNotifConfig
			WebsocketUri:        "ws://test.org",
			RequestWebsocketUri: true,
		},
	}
	body, err = json.Marshal(sensorDiscoveryEventSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_discovery", bytes.NewBuffer(body), nil, nil, nil, http.StatusInternalServerError, SensorDiscoverySubscriptionPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Received expected response")

	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestSensorDiscoveryEventSubscriptionGET_no_query(t *testing.T) {
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

	sensorDiscoveryEventSubscription, err := createSensorDiscoveryEventSubscription_no_query()
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sensorDiscoveryEventSubscription: ", sensorDiscoveryEventSubscription)

	/******************************
	 * expected response section
	 ******************************/
	var expected = SubscriptionLinkList{
		Links: &SubscriptionLinkListLinks{
			Self: &LinkType{
				Href: "http://localhost/testScenario/sens/v1/subscriptions",
			},
		},
	}
	expected.Links.Subscriptions = append(expected.Links.Subscriptions, SubscriptionLinkListSubscription{Href: "http://localhost/testScenario/sens/v1/subscriptions/2", SubscriptionType: "SensorDiscoveryEventSubscription"})

	/******************************
	 * request vars section
	 ******************************/
	//vars := make(map[string]string)
	//vars["subscriptionId"] = subscription_id

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/sens/v1/subscriptions/sensor_discovery", nil, nil, nil, nil, http.StatusOK, SensorDiscoverySubscriptionGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp SubscriptionLinkList
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("resp", resp)
	if !validateSubscriptionLinkList(expected, resp) {
		t.Fatalf("SubscriptionLinkList mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = deleteSensorDiscoveryEventSubscription(sensorDiscoveryEventSubscription)
	terminateScenario()
}

func TestSensorDiscoveryEventSubscriptionDELETE(t *testing.T) {
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

	sensorDiscoveryEventSubscription, err := createSensorDiscoveryEventSubscription_no_query()
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sensorDiscoveryEventSubscription: ", sensorDiscoveryEventSubscription)
	selfUrl := strings.Split(sensorDiscoveryEventSubscription.Links.Self.Href, "/")
	subscriptionId := selfUrl[len(selfUrl)-1]
	fmt.Println("subscriptionId = ", subscriptionId)

	/******************************
	 * expected response section
	 ******************************/

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["subscriptionId"] = subscriptionId

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodDelete, "/sens/v1/subscriptions/sensor_discovery", nil, vars, nil, nil, http.StatusNoContent, SensorDiscoverySubscriptionDELETE)
	if err != nil {
		t.Fatalf(err.Error())
	}

	fmt.Println("Received expected response")

	_, err = sendRequest(http.MethodGet, "/sens/v1/subscriptions/sensor_discovery", nil, vars, nil, nil, http.StatusNotFound, SensorDiscoveryIndividualSubscriptionGET)
	if err != nil {
		t.Fatalf(err.Error())
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestSensorDiscoveryEventSubscriptionByIdGET_no_query(t *testing.T) {
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
	expected, err := createSensorDiscoveryEventSubscription_no_query()
	if err != nil {
		t.Fatalf(err.Error())
	}
	jsonInfo, err := json.Marshal(&expected)
	if err != nil {
		t.Fatalf(err.Error())
	}
	expected_response := string(jsonInfo)
	fmt.Println("expected_response: ", expected_response)
	selfUrl := strings.Split(expected.Links.Self.Href, "/")
	subscriptionId := selfUrl[len(selfUrl)-1]
	fmt.Println("subscriptionId = ", subscriptionId)

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["subscriptionId"] = subscriptionId

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/sens/v1/subscriptions/sensor_discovery", nil, vars, nil, nil, http.StatusOK, SensorDiscoveryIndividualSubscriptionGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp SensorDiscoveryEventSubscription
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !validateSensorDiscoveryEventSubscription(expected, resp) {
		t.Fatalf("handler returned unexpected body: got %v want %v", rr, expected_response)
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = deleteSensorDiscoveryEventSubscription(expected /*resp*/)
	terminateScenario()
}

func createSensorDiscoveryEventSubscription_no_query() (sensorDiscoveryEventSubscription SensorDiscoveryEventSubscription, err error) {
	subscriptionType := SENSOR_DISCOVERY_EVENT_SUBSCRIPTION
	sensorDiscoveryEventSubscription = SensorDiscoveryEventSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	body, err := json.Marshal(sensorDiscoveryEventSubscription)
	if err != nil {
		return sensorDiscoveryEventSubscription, err
	}

	rr, err := sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_discovery", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, SensorDiscoverySubscriptionPOST)
	if err != nil {
		return sensorDiscoveryEventSubscription, err
	}
	err = json.Unmarshal([]byte(rr), &sensorDiscoveryEventSubscription)
	if err != nil {
		return sensorDiscoveryEventSubscription, err
	}

	return sensorDiscoveryEventSubscription, nil
}

func deleteSensorDiscoveryEventSubscription(sensorDiscoveryEventSubscription SensorDiscoveryEventSubscription) (err error) {

	selfUrl := strings.Split(sensorDiscoveryEventSubscription.Links.Self.Href, "/")
	subscriptionId := selfUrl[len(selfUrl)-1]
	fmt.Println("subscriptionId = ", subscriptionId)

	vars := make(map[string]string)
	vars["subscriptionId"] = subscriptionId

	_, err = sendRequest(http.MethodDelete, "/sens/v1/subscriptions/sensor_discovery", nil, vars, nil, nil, http.StatusNoContent, SensorDiscoverySubscriptionDELETE)
	if err != nil {
		return err
	}

	return nil
}

func validateSensorDiscoveryEventSubscription(expected SensorDiscoveryEventSubscription, resp SensorDiscoveryEventSubscription) bool {
	fmt.Println(">>> validateSensorDiscoveryEventSubscription: expected: ", expected)
	fmt.Println(">>> validateSensorDiscoveryEventSubscription: resp: ", resp)

	if *expected.SubscriptionType != *resp.SubscriptionType {
		fmt.Println("expected.SubscriptionType != resp.SubscriptionType")
		return false
	}
	if expected.CallbackReference != resp.CallbackReference {
		fmt.Println("expected.SubscriptionType != resp.SubscriptionType")
		return false
	}
	if expected.RequestTestNotification != resp.RequestTestNotification {
		fmt.Println("expected.RequestTestNotification != resp.RequestTestNotification")
		return false
	}

	return true
}

func TestSensorStatusSubscriptionPOST_no_query(t *testing.T) {
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
	subscriptionType := SENSOR_STATUS_SUBSCRIPTION
	var expected = SensorStatusSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	jsonInfo, err := json.Marshal(&expected)
	if err != nil {
		t.Fatalf(err.Error())
	}
	expected_response := string(jsonInfo)
	fmt.Println("expected_response: ", expected_response)

	/******************************
	 * request vars section
	 ******************************/
	//vars := make(map[string]string)
	//vars["subscriptionId"] = subscription_id

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/
	var sensorStatusSubscription = SensorStatusSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	body, err := json.Marshal(sensorStatusSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_status", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, SensorStatusSubscriptionPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp SensorStatusSubscription
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !validateSensorStatusSubscription(expected, resp) {
		t.Fatalf("handler returned unexpected body: got %v want %v", rr, expected_response)
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = deleteSensorStatusSubscription(resp)
	terminateScenario()
}

func TestSensorStatusSubscriptionGET_no_query(t *testing.T) {
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

	sensorStatusSubscription, err := createSensorStatusSubscription_no_query()
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sensorStatusSubscription: ", sensorStatusSubscription)

	/******************************
	 * expected response section
	 ******************************/
	var expected = SubscriptionLinkList{
		Links: &SubscriptionLinkListLinks{
			Self: &LinkType{
				Href: "http://localhost/testScenario/sens/v1/subscriptions",
			},
		},
	}
	expected.Links.Subscriptions = append(expected.Links.Subscriptions, SubscriptionLinkListSubscription{Href: "http://localhost/testScenario/sens/v1/subscriptions/6", SubscriptionType: "SensorStatusSubscription"})

	/******************************
	 * request vars section
	 ******************************/
	//vars := make(map[string]string)
	//vars["subscriptionId"] = subscription_id

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/sens/v1/subscriptions/sensor_status", nil, nil, nil, nil, http.StatusOK, SensorStatusSubscriptionGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp SubscriptionLinkList
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("resp", resp)
	if !validateSubscriptionLinkList(expected, resp) {
		t.Fatalf("SubscriptionLinkList mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = deleteSensorStatusSubscription(sensorStatusSubscription)
	terminateScenario()
}

func createSensorStatusSubscription_no_query() (sensorStatusSubscription SensorStatusSubscription, err error) {
	subscriptionType := SENSOR_STATUS_SUBSCRIPTION
	sensorStatusSubscription = SensorStatusSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	body, err := json.Marshal(sensorStatusSubscription)
	if err != nil {
		return sensorStatusSubscription, err
	}

	rr, err := sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_status", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, SensorStatusSubscriptionPOST)
	if err != nil {
		return sensorStatusSubscription, err
	}
	err = json.Unmarshal([]byte(rr), &sensorStatusSubscription)
	if err != nil {
		return sensorStatusSubscription, err
	}

	return sensorStatusSubscription, nil
}

func deleteSensorStatusSubscription(sensorStatusSubscription SensorStatusSubscription) (err error) {

	selfUrl := strings.Split(sensorStatusSubscription.Links.Self.Href, "/")
	subscriptionId := selfUrl[len(selfUrl)-1]
	fmt.Println("subscriptionId = ", subscriptionId)

	vars := make(map[string]string)
	vars["subscriptionId"] = subscriptionId

	_, err = sendRequest(http.MethodDelete, "/sens/v1/subscriptions/sensor_status", nil, vars, nil, nil, http.StatusNoContent, SensorDiscoverySubscriptionDELETE)
	if err != nil {
		return err
	}

	return nil
}

func validateSensorStatusSubscription(expected SensorStatusSubscription, resp SensorStatusSubscription) bool {
	fmt.Println(">>> validateSensorStatusSubscription: expected: ", expected)
	fmt.Println(">>> validateSensorStatusSubscription: resp: ", resp)

	if *expected.SubscriptionType != *resp.SubscriptionType {
		fmt.Println("expected.SubscriptionType != resp.SubscriptionType")
		return false
	}
	if expected.CallbackReference != resp.CallbackReference {
		fmt.Println("expected.SubscriptionType != resp.SubscriptionType")
		return false
	}
	if expected.RequestTestNotification != resp.RequestTestNotification {
		fmt.Println("expected.RequestTestNotification != resp.RequestTestNotification")
		return false
	}

	return true
}

func TestSensorDataSubscriptionPOST_no_query(t *testing.T) {
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
	subscriptionType := SENSOR_STATUS_SUBSCRIPTION
	var expected = SensorDataSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	jsonInfo, err := json.Marshal(&expected)
	if err != nil {
		t.Fatalf(err.Error())
	}
	expected_response := string(jsonInfo)
	fmt.Println("expected_response: ", expected_response)

	/******************************
	 * request vars section
	 ******************************/
	//vars := make(map[string]string)
	//vars["subscriptionId"] = subscription_id

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/
	var sensorDataSubscription = SensorDataSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	body, err := json.Marshal(sensorDataSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_data", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, SensorDataSubscriptionPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp SensorDataSubscription
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !validateSensorDataSubscription(expected, resp) {
		t.Fatalf("handler returned unexpected body: got %v want %v", rr, expected_response)
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = deleteSensorDataSubscription(resp)
	terminateScenario()
}

func TestSensorDataSubscriptionGET_no_query(t *testing.T) {
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

	sensorDataSubscription, err := createSensorDataSubscription_no_query()
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sensorDataSubscription: ", sensorDataSubscription)

	/******************************
	 * expected response section
	 ******************************/
	var expected = SubscriptionLinkList{
		Links: &SubscriptionLinkListLinks{
			Self: &LinkType{
				Href: "http://localhost/testScenario/sens/v1/subscriptions",
			},
		},
	}
	expected.Links.Subscriptions = append(expected.Links.Subscriptions, SubscriptionLinkListSubscription{Href: "http://localhost/testScenario/sens/v1/subscriptions/8", SubscriptionType: "SensorDataSubscription"})

	/******************************
	 * request vars section
	 ******************************/
	//vars := make(map[string]string)
	//vars["subscriptionId"] = subscription_id

	/******************************
	 * request queries section
	 ******************************/
	//queries := make(map[string]string)
	//queries["sensorIdentifier"] = "4,5,6"

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/sens/v1/subscriptions/sensor_data", nil, nil, nil, nil, http.StatusOK, SensorDataSubscriptionGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp SubscriptionLinkList
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("resp", resp)
	if !validateSubscriptionLinkList(expected, resp) {
		t.Fatalf("SubscriptionLinkList mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = deleteSensorDataSubscription(sensorDataSubscription)
	terminateScenario()
}

func createSensorDataSubscription_no_query() (sensorDataSubscription SensorDataSubscription, err error) {
	subscriptionType := SENSOR_DATA_SUBSCRIPTION
	sensorDataSubscription = SensorDataSubscription{
		SubscriptionType:  &subscriptionType,
		CallbackReference: "http://test.org",
	}
	body, err := json.Marshal(sensorDataSubscription)
	if err != nil {
		return sensorDataSubscription, err
	}

	rr, err := sendRequest(http.MethodPost, "/sens/v1/subscriptions/sensor_data", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, SensorDataSubscriptionPOST)
	if err != nil {
		return sensorDataSubscription, err
	}
	err = json.Unmarshal([]byte(rr), &sensorDataSubscription)
	if err != nil {
		return sensorDataSubscription, err
	}

	return sensorDataSubscription, nil
}

func deleteSensorDataSubscription(sensorDataSubscription SensorDataSubscription) (err error) {

	selfUrl := strings.Split(sensorDataSubscription.Links.Self.Href, "/")
	subscriptionId := selfUrl[len(selfUrl)-1]
	fmt.Println("subscriptionId = ", subscriptionId)

	vars := make(map[string]string)
	vars["subscriptionId"] = subscriptionId

	_, err = sendRequest(http.MethodDelete, "/sens/v1/subscriptions/sensor_data", nil, vars, nil, nil, http.StatusNoContent, SensorDiscoverySubscriptionDELETE)
	if err != nil {
		return err
	}

	return nil
}

func validateSensorDataSubscription(expected SensorDataSubscription, resp SensorDataSubscription) bool {
	fmt.Println(">>> validateSensorDataSubscription: expected: ", expected)
	fmt.Println(">>> validateSensorDataSubscription: resp: ", resp)

	if *expected.SubscriptionType != *resp.SubscriptionType {
		fmt.Println("expected.SubscriptionType != resp.SubscriptionType")
		return false
	}
	if expected.CallbackReference != resp.CallbackReference {
		fmt.Println("expected.SubscriptionType != resp.SubscriptionType")
		return false
	}
	if expected.RequestTestNotification != resp.RequestTestNotification {
		fmt.Println("expected.RequestTestNotification != resp.RequestTestNotification")
		return false
	}

	return true
}

func validateSubscriptionLinkList(expected SubscriptionLinkList, resp SubscriptionLinkList) bool {
	fmt.Println(">>> validateSubscriptionLinkList: expected: ", expected)
	fmt.Println(">>> validateSubscriptionLinkList: resp: ", resp)

	if expected.Links.Self.Href != resp.Links.Self.Href {
		fmt.Println("expected.Links.Self.Href != resp.Links.Self.Href")
		return false
	}
	if len(expected.Links.Subscriptions) != len(resp.Links.Subscriptions) {
		fmt.Println("expected.Links.Subscriptions length mismatch")
		fmt.Println("len(expected.Links.Subscriptions): ", len(expected.Links.Subscriptions))
		fmt.Println("len(resp.Links.Subscriptions: ", len(resp.Links.Subscriptions))
		return false
	} else {
		for i, sub := range expected.Links.Subscriptions {
			if sub.Href != resp.Links.Subscriptions[i].Href {
				fmt.Println("sub.Href != resp.Links.Subscriptions[i].Href")
				return false
			}
			if sub.SubscriptionType != resp.Links.Subscriptions[i].SubscriptionType {
				fmt.Println("sub.SubscriptionType != resp.Links.Subscriptions[i].SubscriptionType")
				fmt.Println("Index: ", i)
				fmt.Println("sub.SubscriptionType: ", sub.SubscriptionType)
				fmt.Println("resp.Links.Subscriptions[i].SubscriptionType: ", resp.Links.Subscriptions[i].SubscriptionType)
				return false
			}
		}
	}

	return true
}

func initializeVars() {
	mod.DbAddress = redisTestAddr
	redisAddr = redisTestAddr
	influxAddr = influxTestAddr
	sandboxName = testScenarioName
	os.Setenv("MEEP_PREDICT_MODEL_SUPPORTED", "true")
	os.Setenv("MEEP_SANDBOX_NAME", testScenarioName)
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
			return "", fmt.Errorf("Header Location expected")
		} else if !strings.Contains(s, *location) {
			s := fmt.Sprintf("Wrong Header Location - got %s want %s", s, *location)
			return "", errors.New(s)
		}
	}

	return string(rr.Body.String()), nil
}
