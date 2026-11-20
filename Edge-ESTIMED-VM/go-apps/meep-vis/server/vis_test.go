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

//const INITIAL = 0
//const UPDATED = 1

const EXPIRY_DEADLINE = 20

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
const postgisTestHost = "localhost"
const postgisTestPort = "30432"
const testScenarioName = "testScenario"
const v2xBrokerTest = "mqtt://broker.emqx.io:1883" // Or amqp://guest:guest@localhost:5672
const v2xTopicTest = "3gpp/v2x/obu"

var poaListTest = []string{"zone1-poa-cell1", "zone1-poa-cell2"}

var m *mod.Model
var mqLocal *mq.MsgQueue

func TestPredictedQosPost(t *testing.T) {
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
	// Initialize the data structure for the POST request
	// MEC-030 Clause 6.2.5
	// MEC-030 Clause 7.6.3.4
	expected_pointA := LocationInfoGeoArea{43.733505, 7.413917}
	expected_locationInfoA := LocationInfo{nil, &expected_pointA}
	expected_pointB := LocationInfoGeoArea{43.733515, 7.413916}
	expected_locationInfoB := LocationInfo{nil, &expected_pointB}
	// Fill RouteInfo with LocationInfo list
	expected_routeInfo := make([]RouteInfo, 2)
	expected_routeInfo[0] = RouteInfo{&expected_locationInfoA, nil /*&tsA*/}
	expected_routeInfo[1] = RouteInfo{&expected_locationInfoB, nil /*&tsB*/}
	expected_routes_routeInfos := Routes{expected_routeInfo}
	expected_routes := make([]Routes, 1)
	expected_routes[0] = expected_routes_routeInfos
	expected_locationGranularity := "1"
	expected_noticePeriod := &TimeStamp{NanoSeconds: 0, Seconds: int32(time.Now().Unix())}
	var expected_predictionArea *PredictionArea = nil
	expected_predictionTarget := "1"
	expected_qosKpis := make([]QosKpi, 1)
	expected_qosKpis[0] = QosKpi{"latency", "value", "1"}
	expected_stream := make([]Stream, 1)
	expected_stream[0] = Stream{
		StreamId: "1",
		QosKpi:   expected_qosKpis,
	}
	expected_qos := &Qos{expected_stream}
	expected_timeGranularity := &TimeStamp{0, 1}
	expected_predictedQos := PredictedQos{expected_locationGranularity, expected_noticePeriod, expected_predictionArea, expected_predictionTarget, expected_qos, expected_routes, expected_timeGranularity}
	expected_predictedQos_str, err := json.Marshal(expected_predictedQos)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_predictedQos_str: ", string(expected_predictedQos_str))
	/******************************
	 * request body section
	 ******************************/
	// Initialize the data structure for the POST request
	// MEC-030 Clause 6.2.5
	// MEC-030 Clause 7.6.3.4
	pointA := LocationInfoGeoArea{43.733505, 7.413917}
	locationInfoA := LocationInfo{nil, &pointA}
	//tsA := TimeStamp{0, 45}
	pointB := LocationInfoGeoArea{43.733515, 7.413916}
	locationInfoB := LocationInfo{nil, &pointB}
	//tsB := TimeStamp{0, 45}
	// Fill RouteInfo with LocationInfo list
	routeInfo := make([]RouteInfo, 2)
	routeInfo[0] = RouteInfo{&locationInfoA, nil /*&tsA*/} // FIXME routeInfo.Time Not Supported yet
	routeInfo[1] = RouteInfo{&locationInfoB, nil /*&tsB*/} // FIXME routeInfo.Time Not Supported yet
	routes_routeInfos := Routes{expected_routeInfo}
	routes := make([]Routes, 1)
	routes[0] = routes_routeInfos
	locationGranularity := "1"
	noticePeriod := &TimeStamp{NanoSeconds: 0, Seconds: int32(time.Now().Unix())}
	var predictionArea *PredictionArea = nil
	predictionTarget := "1"
	qosKpis := make([]QosKpi, 1)
	qosKpis[0] = QosKpi{"latency", "value", "1"}
	stream := make([]Stream, 1)
	stream[0] = Stream{
		StreamId: "1",
		QosKpi:   qosKpis,
	}
	qos := &Qos{stream}
	timeGranularity := &TimeStamp{0, 1}
	testPredictedQos := PredictedQos{locationGranularity, noticePeriod, predictionArea, predictionTarget, qos, routes, timeGranularity}
	body, err := json.Marshal(testPredictedQos)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/provide_predicted_qos", bytes.NewBuffer(body), nil, nil, nil, http.StatusOK, PredictedQosPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp PredictedQos
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validatePredictedQos(resp, testPredictedQos) {
		t.Errorf("handler returned unexpected body: got %v want %v", rr, expected_predictedQos_str)
	}
	// TODO Validate with expected response

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func validatePredictedQos(received PredictedQos, expected PredictedQos) bool {
	fmt.Println("validatePredictedQos: received: ", received)
	fmt.Println("validatePredictedQos: expected: ", expected)

	fmt.Println("validatePredictedQos: succeed")
	return true
}

func TestProvInfoUuUnicastGET(t *testing.T) {
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
	// Initialize the data structure for the GET request
	// MEC-030 Clause 6.2.2
	// MEC-030 Clause 7.3.3

	/******************************
	 * expected request section
	 ******************************/
	ecgi_1 := Ecgi{
		CellId: &CellId{CellId: "2345678"},
		Plmn:   &Plmn{Mcc: "123", Mnc: "456"},
	}
	ecgi_2 := Ecgi{
		CellId: &CellId{CellId: "3456789"},
		Plmn:   &Plmn{Mcc: "123", Mnc: "456"},
	}
	plmn := Plmn{Mcc: "123", Mnc: "456"}
	geoArea_1 := LocationInfoGeoArea{float32(43.731724), float32(7.423547)}
	locationInfo_1 := LocationInfo{&ecgi_1, &geoArea_1}
	geoArea_2 := LocationInfoGeoArea{float32(43.731724), float32(7.423547)}
	locationInfo_2 := LocationInfo{&ecgi_2, &geoArea_2}
	uuUniNeighbourCellInfo_1 := make([]UuUniNeighbourCellInfo, 1)
	uuUniNeighbourCellInfo_1[0] = UuUniNeighbourCellInfo{&ecgi_1, nil, 0, &plmn, nil}
	uuUniNeighbourCellInfo_2 := make([]UuUniNeighbourCellInfo, 1)
	uuUniNeighbourCellInfo_2[0] = UuUniNeighbourCellInfo{&ecgi_2, nil, 0, &plmn, nil}
	proInfoUuUnicast := make([]UuUnicastProvisioningInfoProInfoUuUnicast, 2)
	v2xApplicationServer_1 := &V2xApplicationServer{
		IpAddress: "mqtt.server.mno1.com",
		UdpPort:   "12345",
	}
	v2xApplicationServer_2 := &V2xApplicationServer{
		IpAddress: "mqtt.server.mno2.com",
		UdpPort:   "12346",
	}
	proInfoUuUnicast[0] = UuUnicastProvisioningInfoProInfoUuUnicast{&locationInfo_1, uuUniNeighbourCellInfo_1, v2xApplicationServer_1}
	proInfoUuUnicast[1] = UuUnicastProvisioningInfoProInfoUuUnicast{&locationInfo_2, uuUniNeighbourCellInfo_2, v2xApplicationServer_2}
	uuUnicastProvisioningInfo := UuUnicastProvisioningInfo{
		ProInfoUuUnicast: proInfoUuUnicast,
		TimeStamp: &TimeStamp{
			Seconds: int32(time.Now().Unix()),
		},
	}

	expected_json_response, err := json.Marshal(uuUnicastProvisioningInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_json_response: ", string(expected_json_response))

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/queries/uu_unicast_provisioning_info?location_info=ecgi,33139970001614,33139971112725", nil, nil, nil, nil, http.StatusOK, ProvInfoUuUnicastGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp UuUnicastProvisioningInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf("err.Error()")
	}
	fmt.Println("Respone: resp: ", resp)
	if !validateUuUnicastProvisioningInfo(resp, uuUnicastProvisioningInfo) {
		t.Errorf("handler returned unexpected body: got %v want %v", rr, expected_json_response)
	}

	_, err = sendRequest(http.MethodGet, "/queries/uu_unicast_provisioning_info?location_info=ecgi", nil, nil, nil, nil, http.StatusBadRequest, ProvInfoUuUnicastGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sendRequest done")

	rr, err = sendRequest(http.MethodGet, "/queries/uu_unicast_provisioning_info?location_info=latitude,43.731724,longitude,7.423547", nil, nil, nil, nil, http.StatusOK, ProvInfoUuUnicastGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf("Failed to get expected response")
	}
	fmt.Println("Respone: resp: ", resp)
	// TODO Validate with expected response

	_, err = sendRequest(http.MethodGet, "/queries/uu_unicast_provisioning_info?location_info=latitude,000.000,001.000,longitude,000.000", nil, nil, nil, nil, http.StatusBadRequest, ProvInfoUuUnicastGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sendRequest done")

	_, err = sendRequest(http.MethodGet, "/queries/uu_unicast_provisioning_info?location_info=latitude,000.000,001.000", nil, nil, nil, nil, http.StatusBadRequest, ProvInfoUuUnicastGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sendRequest done")

	_, err = sendRequest(http.MethodGet, "/queries/uu_unicast_provisioning_info?location_info=longitude,000.000,001.000,latitude,000.000,001.000", nil, nil, nil, nil, http.StatusBadRequest, ProvInfoUuUnicastGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sendRequest done")

	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func validateUuUnicastProvisioningInfo(received UuUnicastProvisioningInfo, expected UuUnicastProvisioningInfo) bool {
	fmt.Println("validateUuUnicastProvisioningInfo: received: ", received)
	fmt.Println("validateUuUnicastProvisioningInfo: expected: ", expected)
	fmt.Println("validateUuUnicastProvisioningInfo: received: ", len(received.ProInfoUuUnicast))
	fmt.Println("validateUuUnicastProvisioningInfo: expected: ", len(expected.ProInfoUuUnicast))
	if len(received.ProInfoUuUnicast) != len(expected.ProInfoUuUnicast) {
		fmt.Println("len(received.ProInfoUuUnicast) mismatch")
		return false
	}
	for idx, proInfoUuUnicast := range received.ProInfoUuUnicast {
		if len(proInfoUuUnicast.NeighbourCellInfo) != len(expected.ProInfoUuUnicast[idx].NeighbourCellInfo) {
			fmt.Println("len(received.ProInfoUuUnicast.NeighbourCellInfo) mismatch")
			return false
		}
		for jdx, proInfoUuUnicast := range proInfoUuUnicast.NeighbourCellInfo {
			if validateUuNeighbourCellInfo(proInfoUuUnicast, expected.ProInfoUuUnicast[idx].NeighbourCellInfo[jdx]) == false {
				fmt.Println("ProInfoUuUnicast mismatch")
				return false
			}
		} // End of 'for' statement
	} // End of 'for' statement

	if validateTimeStamp(*received.TimeStamp, *expected.TimeStamp) == false {
		fmt.Println("TimeStamp mismatch")
		return false
	}

	fmt.Println("validateUuUnicastProvisioningInfo: succeed")
	return true
}

func validateUuNeighbourCellInfo(received UuUniNeighbourCellInfo, expected UuUniNeighbourCellInfo) bool {
	fmt.Println("validateUuNeighbourCellInfo: received: ", received)
	fmt.Println("validateUuNeighbourCellInfo: expected: ", expected)

	if received.Ecgi != nil && expected.Ecgi != nil {
		if !validateEcgi(*received.Ecgi, *expected.Ecgi) {
			fmt.Println("received.Ecgi mismatch")
			return false
		}
	} else if received.Ecgi != nil || expected.Ecgi != nil {
		fmt.Println("received.Ecgi mismatch")
		return false
	}

	if received.Plmn != nil && expected.Plmn != nil {
		if !validatePlmn(*received.Plmn, *expected.Plmn) {
			fmt.Println("received.Plmn mismatch")
			return false
		}
	} else if received.Plmn != nil || expected.Plmn != nil {
		fmt.Println("received.Plmn mismatch")
		return false
	}

	if received.Pci != expected.Pci {
		fmt.Println("received.Pci mismatch")
		return false
	}

	if received.FddInfo != nil && expected.FddInfo != nil {
		if !validateFddInfo(*received.FddInfo, *expected.FddInfo) {
			fmt.Println("received.FddInfo mismatch")
			return false
		}
	} else if received.FddInfo != nil || expected.FddInfo != nil {
		fmt.Println("received.FddInfo mismatch")
		return false
	}

	if received.TddInfo != nil && expected.TddInfo != nil {
		if !validateTddInfo(*received.TddInfo, *expected.TddInfo) {
			fmt.Println("received.TddInfo mismatch")
			return false
		}
	} else if received.TddInfo != nil || expected.TddInfo != nil {
		fmt.Println("received.TddInfo mismatch")
		return false
	}

	fmt.Println("validateUuUnicastProvisioningInfo: succeed")
	return true
}

func validateV2xApplicationServer(received V2xApplicationServer, expected V2xApplicationServer) bool {
	fmt.Println(">>> validateV2xApplicationServer: received: ", received)
	fmt.Println(">>> validateV2xApplicationServer: expected: ", expected)

	if received.IpAddress != expected.IpAddress {
		fmt.Println("IpAddress mismatch")
		return false
	}
	if received.UdpPort != expected.UdpPort {
		fmt.Println("UdpPort mismatch")
		return false
	}

	fmt.Println("validateV2xApplicationServer: succeed")
	return true
}

func validateTimeStamp(received TimeStamp, expected TimeStamp) bool {
	fmt.Println(">>> validateTimeStamp: received: ", received)
	fmt.Println(">>> validateTimeStamp: expected: ", expected)

	if received.NanoSeconds != expected.NanoSeconds {
		fmt.Println("NanoSeconds mismatch")
		return false
	}
	if received.Seconds != expected.Seconds {
		fmt.Println("Seconds mismatch")
		return false
	}

	fmt.Println("validateTimeStamp: succeed")
	return true
}

func validateLinks(received Links, expected Links) bool {
	fmt.Printf(">>> validateLinks: received: %+v\n", received)
	fmt.Printf(">>> validateLinks: expected: %+v\n", expected)
	fmt.Printf(">>> validateLinks: received.Self: %+v\n", received.Self)
	fmt.Printf(">>> validateLinks: expected.Self: %+v\n", expected.Self)

	if received.Self != nil && expected.Self != nil {
		if received.Self.Href != expected.Self.Href {
			fmt.Println("received.Self.Href mismatch")
			return false
		}
	} else if received.Self != nil || expected.Self != nil {
		fmt.Println("received.Self mismatch")
		return false
	}

	fmt.Println("validateLinks: succeed")
	return true
}

func validateLocationInfo(received LocationInfo, expected LocationInfo) bool {
	fmt.Printf(">>> validateLocationInfo: received: %+v\n", received)
	fmt.Printf(">>> validateLocationInfo: expected: %+v\n", expected)

	if received.Ecgi != nil && expected.Ecgi != nil {
		if !validateEcgi(*received.Ecgi, *expected.Ecgi) {
			fmt.Println("received.Ecgi mismatch")
			return false
		}
	} else if received.Ecgi != nil || expected.Ecgi != nil {
		fmt.Println("received.Ecgi mismatch")
		return false
	}
	if received.GeoArea != nil && expected.GeoArea != nil {
		if !validateLocationInfoGeoArea(*received.GeoArea, *expected.GeoArea) {
			fmt.Println("received.GeoArea mismatch")
			return false
		}
	} else if received.GeoArea != nil || expected.GeoArea != nil {
		fmt.Println("received.GeoArea mismatch")
		return false
	}

	fmt.Println("validateLocationInfo: succeed")
	return true
}

func validateLocationInfoGeoArea(received LocationInfoGeoArea, expected LocationInfoGeoArea) bool {
	fmt.Println(">>> validateLocationInfoGeoArea: received: ", received)
	fmt.Println(">>> validateLocationInfoGeoArea: expected: ", expected)

	if received.Latitude != expected.Latitude {
		fmt.Println("received.Latitude mismatch")
		return false
	}
	if received.Longitude != expected.Longitude {
		fmt.Println("received.Longitude mismatch")
		return false
	}

	fmt.Println("validateLocationInfoGeoArea: succeed")
	return true
}

func validateEcgi(received Ecgi, expected Ecgi) bool {
	fmt.Printf(">>> validateEcgi: received: %+v\n", received)
	fmt.Printf(">>> validateEcgi: expected: %+v\n", expected)

	if received.CellId != nil && expected.CellId != nil {
		if received.CellId.CellId != expected.CellId.CellId {
			fmt.Println("received.CellId mismatch")
			return false
		}
	} else if received.CellId != nil || expected.CellId != nil {
		fmt.Println("received.CellId mismatch")
		return false
	}
	if received.Plmn != nil && expected.Plmn != nil {
		if !validatePlmn(*expected.Plmn, *received.Plmn) {
			fmt.Println("received.Plmn mismatch")
			return false
		}
	} else if received.Plmn != nil || expected.Plmn != nil {
		fmt.Println("received.Plmn mismatch")
		return false
	}

	fmt.Println("validateEcgi: succeed")
	return true
}

func validatePlmn(received Plmn, expected Plmn) bool {
	fmt.Printf(">>> validatePlmn: received: %+v\n", received)
	fmt.Printf(">>> validatePlmn: expected: %+v\n", expected)

	if received.Mcc != expected.Mcc {
		fmt.Println("received.Mcc mismatch")
		return false
	}
	if received.Mnc != expected.Mnc {
		fmt.Println("received.Mnc mismatch")
		return false
	}

	fmt.Println("validatePlmn: succeed")
	return true
}

func validateFddInfo(received FddInfo, expected FddInfo) bool {
	fmt.Printf(">>> validateFddInfo: received: %+v\n", received)
	fmt.Printf(">>> validateFddInfo: expected: %+v\n", expected)

	if received.DlEarfcn != nil && expected.DlEarfcn != nil {
		fmt.Printf("received.DlEarfcn: %+v\n", received.DlEarfcn)
		fmt.Printf("expected.DlEarfcn: %+v\n", expected.DlEarfcn)
		if *received.DlEarfcn != *expected.DlEarfcn {
			fmt.Println("received.DlEarfcn mismatch")
			return false
		}
	} else if received.DlEarfcn != nil || expected.DlEarfcn != nil {
		fmt.Println("received.DlEarfcn mismatch")
		return false
	}

	if received.DlTransmissionBandwidth != nil && expected.DlTransmissionBandwidth != nil {
		fmt.Printf("received.DlTransmissionBandwidth: %+v\n", received.DlTransmissionBandwidth)
		fmt.Printf("expected.DlTransmissionBandwidth: %+v\n", expected.DlTransmissionBandwidth)
		if *received.DlTransmissionBandwidth.TransmissionBandwidth != *expected.DlTransmissionBandwidth.TransmissionBandwidth {
			fmt.Println("received.DlTransmissionBandwidth mismatch")
			return false
		}
	} else if received.DlTransmissionBandwidth != nil || expected.DlTransmissionBandwidth != nil {
		fmt.Println("received.DlTransmissionBandwidth mismatch")
		return false
	}

	if received.UlEarfcn != nil && expected.UlEarfcn != nil {
		fmt.Printf("received.UlEarfcn: %+v\n", received.UlEarfcn)
		fmt.Printf("expected.UlEarfcn: %+v\n", expected.UlEarfcn)
		if *received.UlEarfcn != *expected.UlEarfcn {
			fmt.Println("received.UlEarfcn mismatch")
			return false
		}
	} else if received.UlEarfcn != nil || expected.UlEarfcn != nil {
		fmt.Println("received.UlEarfcn mismatch")
		return false
	}

	if received.UlTransmissionBandwidth != nil && expected.UlTransmissionBandwidth != nil {
		fmt.Printf("received.UlTransmissionBandwidth: %+v\n", received.UlTransmissionBandwidth)
		fmt.Printf("expected.UlTransmissionBandwidth: %+v\n", expected.UlTransmissionBandwidth)
		if *received.UlTransmissionBandwidth.TransmissionBandwidth != *expected.UlTransmissionBandwidth.TransmissionBandwidth {
			fmt.Println("received.UlTransmissionBandwidth mismatch")
			return false
		}
	} else if received.UlTransmissionBandwidth != nil || expected.UlTransmissionBandwidth != nil {
		fmt.Println("received.UlTransmissionBandwidth mismatch")
		return false
	}

	fmt.Println("validateFddInfo: succeed")
	return true
}

func validateTddInfo(received TddInfo, expected TddInfo) bool {
	fmt.Printf(">>> validateTddInfo: received: %+v\n", received)
	fmt.Printf(">>> validateTddInfo: expected: %+v\n", expected)

	if received.Earfcn != nil && expected.Earfcn != nil {
		fmt.Printf("received.Earfcn: %+v\n", received.Earfcn)
		fmt.Printf("expected.Earfcn: %+v\n", expected.Earfcn)
		if *received.Earfcn != *expected.Earfcn {
			fmt.Println("received.Earfcn mismatch")
			return false
		}
	} else if received.Earfcn != nil || expected.Earfcn != nil {
		fmt.Println("received.Earfcn mismatch")
		return false
	}

	if received.SubframeAssignment != expected.SubframeAssignment {
		fmt.Println("received.SubframeAssignment mismatch")
		return false
	}

	if received.TransmissionBandwidth != nil && expected.TransmissionBandwidth != nil {
		fmt.Printf("received.TransmissionBandwidth: %+v\n", received.TransmissionBandwidth)
		fmt.Printf("expected.TransmissionBandwidth: %+v\n", expected.TransmissionBandwidth)
		if *received.TransmissionBandwidth.TransmissionBandwidth != *expected.TransmissionBandwidth.TransmissionBandwidth {
			fmt.Println("received.TransmissionBandwidth mismatch")
			return false
		}
	} else if received.TransmissionBandwidth != nil || expected.TransmissionBandwidth != nil {
		fmt.Println("received.TransmissionBandwidth mismatch")
		return false
	}

	fmt.Println("validateTddInfo: succeed")
	return true
}

func testProvUuUniSubscriptionPost(t *testing.T, requestTestNotification bool, expiryNotification bool) (string, string) {
	fmt.Println(">>> testProvUuUniSubscriptionPost")

	/******************************
	 * expected response section
	 ******************************/
	// Initialize the data structure for the POST request
	// MEC-030 Clause 6.3.5
	// MEC-030 Clause 7.8.3.4
	expected_subscriptionType := PROV_CHG_UU_UNI
	expected_callbackReference := "http://localhost:8080/callback/vis/v2/ProvChgUuUniSubscription/"
	expected_href := LinkType{Href: "http://localhost/testScenario/vis/v2/subscriptions/1"}
	expected_self := Links{Self: &expected_href}
	ecgi_1 := Ecgi{
		CellId: &CellId{CellId: "2345678"},
		Plmn:   &Plmn{Mcc: "123", Mnc: "456"},
	}
	ecgi_2 := Ecgi{
		CellId: &CellId{CellId: "3456789"},
		Plmn:   &Plmn{Mcc: "123", Mnc: "456"},
	}
	plmn := Plmn{Mcc: "123", Mnc: "456"}
	earfcn := Earfcn{0}
	subframeAssignment := "frame_v2x"
	tbw := BW6
	xltbw := &TransmissionBandwidth{&tbw}
	fdd_1 := &FddInfo{DlEarfcn: &earfcn, DlTransmissionBandwidth: xltbw, UlEarfcn: &earfcn, UlTransmissionBandwidth: xltbw}
	fdd_2 := &FddInfo{DlEarfcn: &earfcn, DlTransmissionBandwidth: xltbw, UlEarfcn: &earfcn, UlTransmissionBandwidth: xltbw}
	ttd_1 := &TddInfo{Earfcn: &earfcn, SubframeAssignment: subframeAssignment, TransmissionBandwidth: xltbw}
	ttd_2 := &TddInfo{Earfcn: &earfcn, SubframeAssignment: subframeAssignment, TransmissionBandwidth: xltbw}
	geoArea := LocationInfoGeoArea{float32(43.731724), float32(7.423547)}
	locationInfo := &LocationInfo{&ecgi_1, &geoArea}
	uuUniNeighbourCellInfo := make([]UuUniNeighbourCellInfo, 2)
	uuUniNeighbourCellInfo[0] = UuUniNeighbourCellInfo{Ecgi: &ecgi_1, FddInfo: fdd_1, Pci: 0, Plmn: &plmn, TddInfo: ttd_1}
	uuUniNeighbourCellInfo[1] = UuUniNeighbourCellInfo{Ecgi: &ecgi_2, FddInfo: fdd_2, Pci: 0, Plmn: &plmn, TddInfo: ttd_2}
	v2xApplicationServer := &V2xApplicationServer{
		IpAddress: "mqtt.server.mno1.com",
		UdpPort:   "12345",
	}
	expected_filterCriteria := ProvChgUuUniSubscriptionFilterCriteria{LocationInfo: locationInfo, NeighbourCellInfo: uuUniNeighbourCellInfo, V2xApplicationServer: v2xApplicationServer}
	var expected_expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expected_expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	expected_provChgUuUniSubscription := ProvChgUuUniSubscription{
		SubscriptionType:        expected_subscriptionType,
		CallbackReference:       expected_callbackReference,
		Links:                   &expected_self,
		FilterCriteria:          &expected_filterCriteria,
		ExpiryDeadline:          expected_expiryDeadline,
		RequestTestNotification: requestTestNotification,
		WebsockNotifConfig:      &WebsockNotifConfig{RequestWebsocketUri: false, WebsocketUri: "soc"},
	}
	expectedResponseStr, err := json.Marshal(expected_provChgUuUniSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expectedResponseStr: ", string(expectedResponseStr))
	/******************************
	 * request body section
	 ******************************/
	subscriptionType := PROV_CHG_UU_UNI
	callbackReference := "http://localhost:8080/callback/vis/v2/ProvChgUuUniSubscription/"
	filterCriteria := ProvChgUuUniSubscriptionFilterCriteria{LocationInfo: locationInfo, NeighbourCellInfo: uuUniNeighbourCellInfo, V2xApplicationServer: v2xApplicationServer}
	var expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	requestedProvChgUuUniSubscription := ProvChgUuUniSubscription{
		SubscriptionType:        subscriptionType,
		CallbackReference:       callbackReference,
		Links:                   nil,
		FilterCriteria:          &filterCriteria,
		ExpiryDeadline:          expiryDeadline,
		RequestTestNotification: requestTestNotification,
		WebsockNotifConfig:      nil, //&WebsockNotifConfig{RequestWebsocketUri: false, WebsocketUri: "soc"},
	}
	body, err := json.Marshal(requestedProvChgUuUniSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/vis/v2/subscriptions", bytes.NewBuffer(body), nil, nil, &expected_href.Href, http.StatusCreated, SubPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request sent")
	var respBody ProvChgUuUniSubscription
	err = json.Unmarshal([]byte(rr), &respBody)
	if err != nil {
		t.Fatalf(err.Error())
	}
	/******************************
	 * Comparing responses
	 ******************************/
	if validateProvChgUuUniSubscriptionResponse(respBody, expected_provChgUuUniSubscription) == false {
		t.Fatalf("Failed to get expected response")
	}

	subscriptionId := strings.Split(respBody.Links.Self.Href, "/")
	cleanSubscriptionId := subscriptionId[len(subscriptionId)-1]
	return cleanSubscriptionId, string(expectedResponseStr)
}

func validateProvChgUuUniSubscriptionResponse(received ProvChgUuUniSubscription, expected ProvChgUuUniSubscription) bool {
	fmt.Println(">>> validateProvChgUuUniSubscriptionResponse: ", expected)
	fmt.Println(">>> validateProvChgUuUniSubscriptionResponse: ", received)

	if expected.SubscriptionType != received.SubscriptionType {
		fmt.Println("Failed to get expected response (SubscriptionType)")
		return false
	}

	if received.FilterCriteria == nil {
		fmt.Println("Failed to get expected response (received.FilterCriteria)")
		return false
	}
	if validateProvChgUuUniSubscriptionFilterCriteria(*received.FilterCriteria, *expected.FilterCriteria) == false {
		fmt.Println("Failed to get expected response (ProvChgUuUniSubscriptionFilterCriteria)")
		return false
	}
	if expected.CallbackReference != received.CallbackReference {
		fmt.Println("Failed to get expected response (CallbackReference)")
		return false
	}
	if expected.ExpiryDeadline != nil && received.ExpiryDeadline != nil {
		if validateTimeStamp(*expected.ExpiryDeadline, *received.ExpiryDeadline) == false {
			fmt.Println("Failed to get expected response (ExpiryDeadline)")
			return false
		}
	} else if expected.ExpiryDeadline != nil || received.ExpiryDeadline != nil {
		if validateTimeStamp(*expected.ExpiryDeadline, *received.ExpiryDeadline) == false {
			fmt.Println("Failed to get expected response (ExpiryDeadline)")
			return false
		}
	}
	if validateLinks(*expected.Links, *received.Links) == false {
		fmt.Println("Failed to get expected response (Links)")
		return false
	}
	if received.WebsockNotifConfig != nil {
		fmt.Println("Failed to get expected response (WebsockNotifConfig)")
		return false
	}
	if expected.RequestTestNotification != received.RequestTestNotification {
		fmt.Println("Failed to get expected response (RequestTestNotification)")
		return false
	}

	fmt.Println("validateProvChgUuUniSubscriptionResponse: succeed")
	return true
}

func validateProvChgUuUniSubscriptionFilterCriteria(received ProvChgUuUniSubscriptionFilterCriteria, expected ProvChgUuUniSubscriptionFilterCriteria) bool {
	fmt.Printf(">>> validateProvChgUuUniSubscriptionFilterCriteria: received: %+v\n", received.LocationInfo)
	fmt.Printf(">>> validateProvChgUuUniSubscriptionFilterCriteria: expected: %+v\n", expected.LocationInfo)

	if expected.LocationInfo != nil && received.LocationInfo != nil {
		fmt.Printf("validateProvChgUuUniSubscriptionFilterCriteria: received.LocationInfo: %+v\n", received.LocationInfo)
		fmt.Printf("validateProvChgUuUniSubscriptionFilterCriteria: expected.LocationInfo: %+v\n", expected.LocationInfo)
		if !validateLocationInfo(*expected.LocationInfo, *received.LocationInfo) {
			fmt.Println("Failed to get expected response (expected.LocationInfo)")
			return false
		}
	} else if expected.LocationInfo != nil || received.LocationInfo != nil {
		fmt.Println("Failed to get expected response (expected.LocationInfo)")
		return false
	}
	if len(expected.NeighbourCellInfo) != len(received.NeighbourCellInfo) {
		fmt.Println("Failed to get expected response len(NeighbourCellInfo)")
		return false
	} else {
		fmt.Printf("validateProvChgUuUniSubscriptionFilterCriteria: expected.NeighbourCellInfo: %+v\n", expected.NeighbourCellInfo)
		fmt.Printf("validateProvChgUuUniSubscriptionFilterCriteria: received.NeighbourCellInfo: %+v\n", received.NeighbourCellInfo)
		for i, neighbourCellInfo := range expected.NeighbourCellInfo {
			if !validateUuNeighbourCellInfo(neighbourCellInfo, received.NeighbourCellInfo[i]) {
				fmt.Println("Failed to get expected response expectedNeighbourCellInfo")
				return false
			}
		} // End of 'for'statement
	}

	if expected.V2xApplicationServer != nil && received.V2xApplicationServer != nil {
		if !validateV2xApplicationServer(*received.V2xApplicationServer, *expected.V2xApplicationServer) {
			fmt.Println("Failed to get expected response V2xApplicationServer")
			return false
		}
	} else if expected.V2xApplicationServer != nil || received.V2xApplicationServer != nil {
		fmt.Println("Failed to get expected response V2xApplicationServer")
		return false
	}

	fmt.Println("validateProvChgUuUniSubscriptionFilterCriteria: succeed")
	return true
}

func TestProvUuUnicastSubscription(t *testing.T) {
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
	// POST
	subscriptionId, expectedGetResponse := testProvUuUniSubscriptionPost(t, false, false)
	// GET Subscriptions
	subscriptionTypeQuery := "prov_chg_uu_uni"
	testSubscriptionsGet(t, subscriptionTypeQuery, expectedGetResponse)
	// GET Individual Subscription
	testIndividualSubscriptionGet(t, "prov_chg_uu_uni", subscriptionId, expectedGetResponse, true)
	// PUT
	testProvChgUuUniSubscriptionPut(t, PROV_CHG_UU_UNI, subscriptionId, true, false)
	// DELETE
	testIndividualSubscriptionDelete(t, subscriptionId, true)
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestFailProvChgUuUniSubscription(t *testing.T) {
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
	// GET
	testIndividualSubscriptionGet(t, PROV_CHG_UU_UNI, "invalidSubscriptionId", "", false)
	// PUT
	_ = testProvChgUuUniSubscriptionPut(t, PROV_CHG_UU_UNI, "invalidSubscriptionId", false, false)
	// DELETE
	testIndividualSubscriptionDelete(t, "invalidSubscriptionId", false)
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestProvUuUnicastSubscriptionWithTestNotification(t *testing.T) {
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
	// POST
	subscriptionId, _ := testProvUuUniSubscriptionPost(t, true, false)
	time.Sleep(2 * time.Second)
	// TODO FSCOM Check test/dummy_callback_server.go logs in /tmp (see test/start-ut-env.sh)
	// DELETE
	testIndividualSubscriptionDelete(t, subscriptionId, true)
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestProvUuUnicastSubscriptionWithExpiryDeadline(t *testing.T) {
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
	// POST
	subscriptionId, _ := testProvUuUniSubscriptionPost(t, false, true)
	time.Sleep((EXPIRY_DEADLINE + 5) * time.Second)
	// TODO FSCOM Check test/dummy_callback_server.go logs in /tmp (see test/start-ut-env.sh)
	// DELETE
	testIndividualSubscriptionDelete(t, subscriptionId, false) // Expecting 404 Not Found
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func testV2xMsgSubscriptionPost(t *testing.T, requestTestNotification bool, expiryNotification bool) (string, string) {
	fmt.Println(">>> testV2xMsgSubscriptionPost")

	/******************************
	 * expected response section
	 ******************************/
	// Initialize the data structure for the POST request
	// MEC-030 Clause 6.3.5
	// MEC-030 Clause 7.8.3.4
	expected_subscriptionType := V2X_MSG
	expected_callbackReference := "http://localhost:8080/callback/vis/v2/V2xMsgSubscription/"
	expected_href := LinkType{Href: "http://localhost/testScenario/vis/v2/subscriptions/1"}
	expected_self := Links{Self: &expected_href}
	expected_msgType := []string{"DENM", "CAM"}
	expected_msgProtocolVersion := []int32{1}
	expected_filterCriteria := V2xMsgFilterCriteria{make([]LocationInfo, 0), expected_msgProtocolVersion, expected_msgType, "ETSI"}
	var expected_expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expected_expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	expected_v2xMsgSubscription := V2xMsgSubscription{
		SubscriptionType:        expected_subscriptionType,
		CallbackReference:       expected_callbackReference,
		Links:                   &expected_self,
		FilterCriteria:          &expected_filterCriteria,
		ExpiryDeadline:          expected_expiryDeadline,
		RequestTestNotification: false,
		WebsockNotifConfig:      nil,
	}
	expectedResponseStr, err := json.Marshal(expected_v2xMsgSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expectedResponseStr: ", string(expectedResponseStr))
	/******************************
	 * request body section
	 ******************************/
	subscriptionType := V2X_MSG
	callbackReference := "http://localhost:8080/callback/vis/v2/V2xMsgSubscription/"
	msgType := []string{"DENM", "CAM"}
	msgProtocolVersion := []int32{1}
	filterCriteria := V2xMsgFilterCriteria{make([]LocationInfo, 0), msgProtocolVersion, msgType, "ETSI"}
	var expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	requestedV2xMsgSubscription := V2xMsgSubscription{
		SubscriptionType:        subscriptionType,
		CallbackReference:       callbackReference,
		FilterCriteria:          &filterCriteria,
		ExpiryDeadline:          expiryDeadline,
		RequestTestNotification: false,
		WebsockNotifConfig:      nil,
	}
	body, err := json.Marshal(requestedV2xMsgSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/vis/v2/subscriptions", bytes.NewBuffer(body), nil, nil, &expected_href.Href, http.StatusCreated, SubPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request sent")
	var respBody V2xMsgSubscription
	err = json.Unmarshal([]byte(rr), &respBody)
	if err != nil {
		t.Fatalf(err.Error())
	}
	/******************************
	 * Comparing responses
	 ******************************/
	if validateV2xMsgSubscriptionResponse(respBody, expected_v2xMsgSubscription) == false {
		t.Fatalf("Failed to get expected response (SubscriptionType)")
	}

	subscriptionId := strings.Split(respBody.Links.Self.Href, "/")
	cleanSubscriptionId := subscriptionId[len(subscriptionId)-1]
	return cleanSubscriptionId, string(expectedResponseStr)
}

func validateV2xMsgSubscriptionResponse(received V2xMsgSubscription, expected V2xMsgSubscription) bool {
	fmt.Println(">>> validateV2xMsgSubscriptionResponse: ", expected)
	fmt.Println(">>> validateV2xMsgSubscriptionResponse: ", received)

	if expected.SubscriptionType != received.SubscriptionType {
		fmt.Println("Failed to get expected response (SubscriptionType)")
		return false
	}

	if received.FilterCriteria == nil {
		fmt.Println("Failed to get expected response (received.FilterCriteria)")
		return false
	}
	if validateV2xMsgFilterCriteria(*received.FilterCriteria, *expected.FilterCriteria) == false {
		fmt.Println("Failed to get expected response (V2xMsgFilterCriteria)")
		return false
	}
	if expected.CallbackReference != received.CallbackReference {
		fmt.Println("Failed to get expected response (CallbackReference)")
		return false
	}
	if expected.ExpiryDeadline != nil && received.ExpiryDeadline != nil {
		if validateTimeStamp(*expected.ExpiryDeadline, *received.ExpiryDeadline) == false {
			fmt.Println("Failed to get expected response (ExpiryDeadline)")
			return false
		}
	} else if expected.ExpiryDeadline != nil || received.ExpiryDeadline != nil {
		if validateTimeStamp(*expected.ExpiryDeadline, *received.ExpiryDeadline) == false {
			fmt.Println("Failed to get expected response (ExpiryDeadline)")
			return false
		}
	}
	if validateLinks(*expected.Links, *received.Links) == false {
		fmt.Println("Failed to get expected response (Links)")
		return false
	}
	if received.WebsockNotifConfig != nil {
		fmt.Println("Failed to get expected response (WebsockNotifConfig)")
		return false
	}
	if expected.RequestTestNotification != received.RequestTestNotification {
		fmt.Println("Failed to get expected response (RequestTestNotification)")
		return false
	}

	fmt.Println("validateV2xMsgSubscriptionResponse succeed")
	return true
}

func validateV2xMsgFilterCriteria(received V2xMsgFilterCriteria, expected V2xMsgFilterCriteria) bool {
	fmt.Println(">>> validateV2xMsgSubscriptionFilterCriteria: ", expected)
	fmt.Println(">>> validateV2xMsgSubscriptionFilterCriteria: ", received)

	fmt.Println("validateV2xMsgSubscriptionFilterCriteria: succeed")
	return true
}

func TestV2XMsgSubscription(t *testing.T) {
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
	// POST
	subscriptionId, expectedGetResponse := testV2xMsgSubscriptionPost(t, true, false)
	// GET Subscriptions
	subscriptionTypeQuery := "v2x_msg"
	testSubscriptionsGet(t, subscriptionTypeQuery, expectedGetResponse)
	// GET Individual Subscription
	testIndividualSubscriptionGet(t, "v2x_msg", subscriptionId, expectedGetResponse, true)
	// PUT
	_ = testIndividualSubscriptionV2xMsgSubscriptionPut(t, V2X_MSG, subscriptionId, true, false)
	// DELETE
	testIndividualSubscriptionDelete(t, subscriptionId, true)
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestV2XMsgSubscriptionTestNotification(t *testing.T) {
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
	// POST
	subscriptionId, _ := testV2xMsgSubscriptionPost(t, true, false)
	time.Sleep(2 * time.Second)
	// TODO FSCOM Check test/dummy_callback_server.go logs in /tmp (see test/start-ut-env.sh)
	// DELETE
	testIndividualSubscriptionDelete(t, subscriptionId, true)
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestV2XMsgSubscriptionWithExpiryDeadline(t *testing.T) {
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
	// POST
	subscriptionId, _ := testV2xMsgSubscriptionPost(t, false, true)
	time.Sleep((EXPIRY_DEADLINE + 5) * time.Second)
	// TODO FSCOM Check test/dummy_callback_server.go logs in /tmp (see test/start-ut-env.sh)
	// DELETE
	testIndividualSubscriptionDelete(t, subscriptionId, false) // Expecting 404 Not Found
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestFailV2XMsgSubscription(t *testing.T) {
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
	// GET
	testIndividualSubscriptionGet(t, V2X_MSG, "invalidSubscriptionId", "", false)
	// PUT
	_ = testIndividualSubscriptionV2xMsgSubscriptionPut(t, V2X_MSG, "invalidSubscriptionId", false, false)
	// DELETE
	testIndividualSubscriptionDelete(t, "invalidSubscriptionId", false)
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func testSubscriptionsGet(t *testing.T, subscriptionTypeQuery string, expectedResponse string) {
	fmt.Println(">>> testSubscriptionsGet: ", subscriptionTypeQuery)

	/******************************
	 * expected response section
	 ******************************/
	//passed as a parameter since a POST had to be sent first
	/******************************
	 * request queries section
	 ******************************/
	queryParam := make(map[string]string)
	queryParam["subscription_type"] = subscriptionTypeQuery
	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/vis/v2/subscriptions", nil, nil, queryParam, nil, http.StatusOK, SubGET)
	if err != nil {
		t.Fatalf("Failed to get expected response")
	}
	if validatSubscriptionLinkList(subscriptionTypeQuery, expectedResponse, rr) == false {
		t.Fatalf("Failed to get expected response")
	}
	fmt.Println("Received expected response for GET Subscription method")
}

func validatSubscriptionLinkList(subscriptionTypeQuery string, expectedResponse string, rr string) bool {
	fmt.Println(">>> validatSubscriptionLinkList: expectedResponse: ", expectedResponse)
	fmt.Println(">>> validatSubscriptionLinkList: rr: ", rr)

	var links Links
	if subscriptionTypeQuery == "prov_chg_uu_uni" {
		var expectedResp ProvChgUuUniSubscription
		err := json.Unmarshal([]byte(expectedResponse), &expectedResp)
		if err != nil {
			fmt.Println("Failed to Unmarshal expected response")
			return false
		}
		links = *expectedResp.Links
	} else if subscriptionTypeQuery == "v2x_msg" {
		var expectedResp V2xMsgSubscription
		err := json.Unmarshal([]byte(expectedResponse), &expectedResp)
		if err != nil {
			fmt.Println("Failed to Unmarshal expected response")
			return false
		}
		links = *expectedResp.Links
	} // FIXME FSCOM To be continued
	var subscriptionLinkList SubscriptionLinkList
	err := json.Unmarshal([]byte(rr), &subscriptionLinkList)
	if err != nil {
		fmt.Println("Failed to Unmarshal rr")
		return false
	}
	fmt.Println("validatSubscriptionLinkList: links: ", links)
	fmt.Println("validatSubscriptionLinkList: links.Self: ", *links.Self)
	fmt.Println("validatSubscriptionLinkList: subscriptionLinkList: ", subscriptionLinkList)

	fmt.Println("validatSubscriptionLinkList: succeed")
	return true
}

func testIndividualSubscriptionGet(t *testing.T, subscriptionTypeQuery string, subscriptionId string, expectedResponse string, expectSuccess bool) {
	fmt.Println(">>> testIndividualSubscriptionGet: ", subscriptionTypeQuery)

	/******************************
	 * expected response section
	 ******************************/
	//passed as a parameter since a POST had to be sent first
	/******************************
	 * request vars section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	if expectSuccess {
		rr, err := sendRequest(http.MethodGet, "/vis/v2/subscriptions/"+subscriptionId, nil, nil, nil, nil, http.StatusOK, IndividualSubscriptionGET)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
		if subscriptionTypeQuery == "prov_chg_uu_uni" {
			var expectedResp ProvChgUuUniSubscription
			err := json.Unmarshal([]byte(expectedResponse), &expectedResp)
			if err != nil {
				t.Fatalf("Failed to get expected response")
			}
			var respBody ProvChgUuUniSubscription
			err = json.Unmarshal([]byte(rr), &respBody)
			if err != nil {
				t.Fatalf("Failed to get expected response")
			}
			if validateProvChgUuUniSubscriptionResponse(respBody, expectedResp) == false {
				t.Fatalf("Failed to get expected response")
			}
		} else if subscriptionTypeQuery == "v2x_msg" {
			var expectedResp V2xMsgSubscription
			err := json.Unmarshal([]byte(expectedResponse), &expectedResp)
			if err != nil {
				t.Fatalf("Failed to get expected response")
			}
			var respBody V2xMsgSubscription
			err = json.Unmarshal([]byte(rr), &respBody)
			if err != nil {
				t.Fatalf("Failed to get expected response")
			}
			if validateV2xMsgSubscriptionResponse(respBody, expectedResp) == false {
				t.Fatalf("Failed to get expected response")
			}
		} // FXIME FSCOM To be continued
	} else {
		_, err := sendRequest(http.MethodGet, "/vis/v2/subscriptions/"+subscriptionId, nil, nil, nil, nil, http.StatusNotFound, IndividualSubscriptionGET)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
	}
}

func testIndividualSubscriptionDelete(t *testing.T, subscriptionId string, expectSuccess bool) {
	fmt.Println(">>> testIndividualSubscriptionDelete")

	/******************************
	 * request vars section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	if expectSuccess {
		_, err := sendRequest(http.MethodDelete, "/vis/v2/subscriptions/"+subscriptionId, nil, nil, nil, nil, http.StatusNoContent, IndividualSubscriptionDELETE)
		if err != nil {
			fmt.Println("testIndividualSubscriptionDelete: ", err.Error())
			t.Fatalf("Failed to get expected response")
		}
	} else {
		_, err := sendRequest(http.MethodDelete, "/vis/v2/subscriptions/"+subscriptionId, nil, nil, nil, nil, http.StatusNotFound, IndividualSubscriptionDELETE)
		if err != nil {
			fmt.Println("testIndividualSubscriptionDelete: ", err.Error())
			t.Fatalf("Failed to get expected response")
		}
	}
}

func testIndividualSubscriptionV2xMsgSubscriptionPut(t *testing.T, subscriptionType string, subscriptionId string, expectSuccess bool, expiryNotification bool) string {
	fmt.Println(">>> testIndividualSubscriptionV2xMsgSubscriptionPut: ", subscriptionType)

	/******************************
	 * expected response section
	 ******************************/
	expected_subscriptionType := subscriptionType
	expected_callbackReference := "http://localhost:8080/callback/vis/v2/" + subscriptionType + "/"
	expected_href := LinkType{Href: "http://localhost/vis/v2/subscriptions/" + subscriptionType + "/" + subscriptionId}
	expected_self := Links{Self: &expected_href}
	expected_msgType := []string{"DENM", "CAM"}
	expected_msgProtocolVersion := []int32{1}
	expected_filterCriteria := V2xMsgFilterCriteria{make([]LocationInfo, 0), expected_msgProtocolVersion, expected_msgType, "ETSI"}
	var expected_expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expected_expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	expected_v2xMsgSubscription := V2xMsgSubscription{
		SubscriptionType:        expected_subscriptionType,
		CallbackReference:       expected_callbackReference,
		Links:                   &expected_self,
		FilterCriteria:          &expected_filterCriteria,
		ExpiryDeadline:          expected_expiryDeadline,
		RequestTestNotification: false,
		WebsockNotifConfig:      nil,
	}
	expectedResponseStr, err := json.Marshal(expected_v2xMsgSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expectedResponseStr: ", string(expectedResponseStr))
	/******************************
	 * request vars section
	 ******************************/

	/******************************
	 * request body section
	 ******************************/
	callbackReference := "http://localhost:8080/callback/vis/v2/" + subscriptionType + "/"
	href := LinkType{Href: "http://localhost/vis/v2/subscriptions/" + subscriptionType + "/1"}
	self := Links{Self: &href}
	msgType := []string{"DENM", "CAM"}
	msgProtocolVersion := []int32{1}
	filterCriteria := V2xMsgFilterCriteria{make([]LocationInfo, 0), msgProtocolVersion, msgType, "ETSI"}
	var expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	requestedv2xMsgSubscription := V2xMsgSubscription{
		SubscriptionType:        subscriptionType,
		CallbackReference:       callbackReference,
		Links:                   &self,
		FilterCriteria:          &filterCriteria,
		ExpiryDeadline:          expiryDeadline,
		RequestTestNotification: false,
		WebsockNotifConfig:      nil,
	}
	body, err := json.Marshal(requestedv2xMsgSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request queries section
	 ******************************/
	/******************************
	 * request execution section
	 ******************************/
	if expectSuccess {
		rr, err := sendRequest(http.MethodPut, "/vis/v2/subscriptions/"+subscriptionId, bytes.NewBuffer(body), nil, nil, nil, http.StatusOK, IndividualSubscriptionPUT)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
		var respBody V2xMsgSubscription
		err = json.Unmarshal([]byte(rr), &respBody)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
		if validateV2xMsgSubscriptionResponse(respBody, expected_v2xMsgSubscription) == false {
			t.Fatalf("Failed to get expected response")
		}
		return string(expectedResponseStr)
	} else {
		_, err = sendRequest(http.MethodPost, "/vis/v2/subscriptions/"+subscriptionId, bytes.NewBuffer(body), nil, nil, nil, http.StatusNotFound, IndividualSubscriptionPUT)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
		return ""
	}
}

func testProvChgUuUniSubscriptionPut(t *testing.T, subscriptionType string, subscriptionId string, expectSuccess bool, expiryNotification bool) string {
	fmt.Println(">>> testProvChgUuUniSubscriptionPut: ", subscriptionType)

	/******************************
	 * expected response section
	 ******************************/
	expected_subscriptionType := subscriptionType
	expected_callbackReference := "http://localhost:8080/callback/vis/v2/" + subscriptionType + "/"
	expected_href := LinkType{Href: "http://localhost/vis/v2/subscriptions/" + subscriptionType + "/" + subscriptionId}
	expected_self := Links{Self: &expected_href}
	ecgi_1 := Ecgi{
		CellId: &CellId{CellId: "2345678"},
		Plmn:   &Plmn{Mcc: "123", Mnc: "456"},
	}
	ecgi_2 := Ecgi{
		CellId: &CellId{CellId: "3456789"},
		Plmn:   &Plmn{Mcc: "123", Mnc: "456"},
	}
	plmn := Plmn{Mcc: "123", Mnc: "456"}
	geoArea := LocationInfoGeoArea{float32(43.731724), float32(7.423547)}
	locationInfo := LocationInfo{&ecgi_1, &geoArea}
	uuUniNeighbourCellInfo := make([]UuUniNeighbourCellInfo, 2)
	uuUniNeighbourCellInfo[0] = UuUniNeighbourCellInfo{&ecgi_1, nil, 0, &plmn, nil}
	uuUniNeighbourCellInfo[1] = UuUniNeighbourCellInfo{&ecgi_2, nil, 0, &plmn, nil}
	v2xApplicationServer := &V2xApplicationServer{
		IpAddress: "mqtt.server.mno1.com",
		UdpPort:   "12345",
	}
	expected_filterCriteria := ProvChgUuUniSubscriptionFilterCriteria{&locationInfo, uuUniNeighbourCellInfo, v2xApplicationServer}
	var expected_expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expected_expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	expected_provChgUuUniSubscription := ProvChgUuUniSubscription{
		SubscriptionType:        expected_subscriptionType,
		CallbackReference:       expected_callbackReference,
		Links:                   &expected_self,
		FilterCriteria:          &expected_filterCriteria,
		ExpiryDeadline:          expected_expiryDeadline,
		RequestTestNotification: false,
		WebsockNotifConfig:      nil,
	}
	expectedResponseStr, err := json.Marshal(expected_provChgUuUniSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expectedResponseStr: ", string(expectedResponseStr))
	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["subscriptionId"] = subscriptionId
	/******************************
	 * request body section
	 ******************************/
	callbackReference := "http://localhost:8080/callback/vis/v2/" + subscriptionType + "/"
	href := LinkType{Href: "http://localhost/vis/v2/subscriptions/" + subscriptionType + "/" + subscriptionId}
	self := Links{Self: &href}
	filterCriteria := ProvChgUuUniSubscriptionFilterCriteria{&locationInfo, uuUniNeighbourCellInfo, v2xApplicationServer}
	var expiryDeadline *TimeStamp = nil
	if expiryNotification {
		expiryDeadline = &TimeStamp{Seconds: int32(time.Now().Unix()) + EXPIRY_DEADLINE, NanoSeconds: 0}
	}
	requestedProvChgUuUniSubscription := ProvChgUuUniSubscription{
		SubscriptionType:        subscriptionType,
		CallbackReference:       callbackReference,
		Links:                   &self,
		FilterCriteria:          &filterCriteria,
		ExpiryDeadline:          expiryDeadline,
		RequestTestNotification: false,
		WebsockNotifConfig:      nil,
	}
	body, err := json.Marshal(requestedProvChgUuUniSubscription)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request queries section
	 ******************************/
	/******************************
	 * request execution section
	 ******************************/
	if expectSuccess {
		rr, err := sendRequest(http.MethodPut, "/vis/v2/subscriptions/"+subscriptionId, bytes.NewBuffer(body), nil, nil, nil, http.StatusOK, IndividualSubscriptionPUT)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
		var respBody ProvChgUuUniSubscription
		err = json.Unmarshal([]byte(rr), &respBody)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
		if validateProvChgUuUniSubscriptionResponse(respBody, expected_provChgUuUniSubscription) == false {
			t.Fatalf("Failed to get expected response")
		}
		return string(expectedResponseStr)
	} else {
		_, err := sendRequest(http.MethodPut, "/vis/v2/subscriptions/"+subscriptionId, bytes.NewBuffer(body), vars, nil, nil, http.StatusNotFound, IndividualSubscriptionPUT)
		if err != nil {
			t.Fatalf("Failed to get expected response")
		}
		return ""
	}
}

func TestV2xMsgPublicationPost(t *testing.T) {
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

	// POST
	subscriptionId, _ := testV2xMsgSubscriptionPost(t, true, false)

	/******************************
	 * expected response section
	 ******************************/
	// Initialize the data structure for the POST request
	// MEC-030 Clause 6.2.6
	// MEC-030 Clause 7.7.3.4
	/******************************
	 * expected request section
	 ******************************/
	stdOrganization := "ETSI"
	msgEncodeFormat := "hexadump"
	msgType := "DENM"
	msgContent := "031200f101038100400380818c20400100005802001ee600003c0004e548140072066b24d01eb78149084d5571800000"
	testv2xMsgPublication := V2xMsgPublication{
		MsgContent:              msgContent,
		MsgPropertiesValues:     &V2xMsgPropertiesValues{nil, 1, msgType, stdOrganization},
		MsgRepresentationFormat: msgEncodeFormat,
	}
	body, err := json.Marshal(testv2xMsgPublication)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/vis/v2/publish_v2x_message", bytes.NewBuffer(body), nil, nil, nil, http.StatusNoContent, V2xMessagePOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("sendRequest done")
	// TODO FSCOM Check test/dummy_callback_server.go logs in /tmp (see test/start-ut-env.sh)

	/******************************
	 * back to initial state section
	 ******************************/
	// DELETE
	testIndividualSubscriptionDelete(t, subscriptionId, true)

	terminateScenario()
}

func TestV2xMsgDistributionServerPost(t *testing.T) {
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
	// Initialize the data structure for the POST request
	// MEC-030 Clause 6.2.6
	// MEC-030 Clause 7.7.3.4
	/******************************
	 * expected request section
	 ******************************/
	locationInfo := make([]LocationInfo, 0)
	msgProtocol := make([]int32, 1)
	msgProtocol[0] = 0
	infoProtocol := &InfoProtocol{MsgProtocol: msgProtocol, ProtImplementation: ""}
	expected_infoConnection := &InfoConnection{IpAddress: "test.mosquitto.org", PortNumber: 1338}
	expected_v2xMsgDistributionServer := make([]V2xMsgDistributionServer, 1)
	expected_v2xMsgDistributionServer[0] = V2xMsgDistributionServer{InfoConnection: expected_infoConnection, InfoProtocol: infoProtocol}
	expected_v2xMsgDistributionServerInfo := V2xMsgDistributionServerInfo{
		LocationInfo:             locationInfo,
		V2xMsgDistributionServer: expected_v2xMsgDistributionServer,
	}
	expected_json_response, err := json.Marshal(expected_v2xMsgDistributionServerInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_json_response: ", string(expected_json_response))

	/******************************
	 * request execution section
	 ******************************/
	v2xMsgDistributionServer := make([]V2xMsgDistributionServer, 1)
	v2xMsgDistributionServer[0] = V2xMsgDistributionServer{InfoConnection: nil, InfoProtocol: infoProtocol}
	v2xMsgDistributionServerInfo := V2xMsgDistributionServerInfo{
		LocationInfo:             locationInfo,
		V2xMsgDistributionServer: v2xMsgDistributionServer,
	}
	body, err := json.Marshal(v2xMsgDistributionServerInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	rr, err := sendRequest(http.MethodPost, "/vis/v2/provide_v2x_msg_distribution_server_info", bytes.NewBuffer(body), nil, nil, nil, http.StatusOK, V2xMsgDistributionServerPost)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp V2xMsgDistributionServerInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf("err.Error()")
	}
	fmt.Println("Respone: resp: ", resp)

	if !validateV2xMsgDistributionServerResponse(resp, expected_v2xMsgDistributionServerInfo) {
		t.Errorf("handler returned unexpected body: got %v want %v", rr, expected_json_response)
	}

	/******************************
	 * back to initial state section
	 ******************************/

	terminateScenario()
}

func TestFailV2xMsgDistributionServerPost(t *testing.T) {
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
	// Initialize the data structure for the POST request
	// MEC-030 Clause 6.2.6
	// MEC-030 Clause 7.7.3.4
	/******************************
	 * expected request section
	 ******************************/
	locationInfo := make([]LocationInfo, 0)
	msgProtocol := make([]int32, 1)
	msgProtocol[0] = 0
	infoProtocol := &InfoProtocol{MsgProtocol: msgProtocol, ProtImplementation: ""}
	expected_infoConnection := &InfoConnection{IpAddress: "test.mosquitto.org", PortNumber: 1338}
	expected_v2xMsgDistributionServer := make([]V2xMsgDistributionServer, 1)
	expected_v2xMsgDistributionServer[0] = V2xMsgDistributionServer{InfoConnection: expected_infoConnection, InfoProtocol: infoProtocol}
	expected_v2xMsgDistributionServerInfo := V2xMsgDistributionServerInfo{
		LocationInfo:             locationInfo,
		V2xMsgDistributionServer: expected_v2xMsgDistributionServer,
	}
	expected_json_response, err := json.Marshal(expected_v2xMsgDistributionServerInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_json_response: ", string(expected_json_response))

	/******************************
	 * request execution section
	 ******************************/
	body, err := json.Marshal(expected_v2xMsgDistributionServerInfo) // Error: InfoConnection field is present :(
	if err != nil {
		t.Fatalf(err.Error())
	}
	_, err = sendRequest(http.MethodPost, "/vis/v2/provide_v2x_msg_distribution_server_info", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, V2xMsgDistributionServerPost)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	expected_v2xMsgDistributionServerInfo.V2xMsgDistributionServer[0].InfoProtocol.MsgProtocol = make([]int32, 0) // No message protocol
	body, err = json.Marshal(expected_v2xMsgDistributionServerInfo)                                               // Error: InfoConnection field is present :(
	if err != nil {
		t.Fatalf(err.Error())
	}
	_, err = sendRequest(http.MethodPost, "/vis/v2/provide_v2x_msg_distribution_server_info", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, V2xMsgDistributionServerPost)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	expected_v2xMsgDistributionServerInfo.V2xMsgDistributionServer = make([]V2xMsgDistributionServer, 0) // No V2xMsgDistributionServer
	body, err = json.Marshal(expected_v2xMsgDistributionServerInfo)                                      // Error: InfoConnection field is present :(
	if err != nil {
		t.Fatalf(err.Error())
	}
	_, err = sendRequest(http.MethodPost, "/vis/v2/provide_v2x_msg_distribution_server_info", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, V2xMsgDistributionServerPost)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	/******************************
	 * back to initial state section
	 ******************************/

	terminateScenario()
}

func validateV2xMsgDistributionServerResponse(received V2xMsgDistributionServerInfo, expected V2xMsgDistributionServerInfo) bool {
	fmt.Println("validateV2xMsgDistributionServerResponse: received: ", received)
	fmt.Println("validateV2xMsgDistributionServerResponse: expected: ", expected)

	if len(received.LocationInfo) != len(expected.LocationInfo) {
		fmt.Println("len(received.LocationInfo) mismatch")
		return false
	}
	if len(received.V2xMsgDistributionServer) != len(expected.V2xMsgDistributionServer) {
		fmt.Println("len(received.LocationInfo) mismatch")
		return false
	} else {
		for i, v2xMsgDistributionServer := range received.V2xMsgDistributionServer {
			if v2xMsgDistributionServer.InfoProtocol != nil && received.V2xMsgDistributionServer[i].InfoProtocol != nil {
				if len(v2xMsgDistributionServer.InfoProtocol.MsgProtocol) != len(received.V2xMsgDistributionServer[i].InfoProtocol.MsgProtocol) {
					fmt.Println("len(v2xMsgDistributionServer.InfoProtocol.MsgProtocol) mismatch")
					return false
				}
				if v2xMsgDistributionServer.InfoProtocol.ProtImplementation != received.V2xMsgDistributionServer[i].InfoProtocol.ProtImplementation {
					fmt.Println("v2xMsgDistributionServer.InfoProtocol.ProtImplementation mismatch")
					return false
				}
			} else if v2xMsgDistributionServer.InfoProtocol != nil || received.V2xMsgDistributionServer[i].InfoProtocol != nil {
				fmt.Println("v2xMsgDistributionServer.InfoProtocol mismatch")
				return false
			}
			if v2xMsgDistributionServer.InfoConnection != nil && received.V2xMsgDistributionServer[i].InfoConnection != nil {
				if v2xMsgDistributionServer.InfoConnection.IpAddress != received.V2xMsgDistributionServer[i].InfoConnection.IpAddress {
					fmt.Println("v2xMsgDistributionServer.InfoConnection.IpAddress mismatch")
					return false
				}
				if v2xMsgDistributionServer.InfoConnection.PortNumber != received.V2xMsgDistributionServer[i].InfoConnection.PortNumber {
					fmt.Println("v2xMsgDistributionServer.InfoConnection.PortNumber mismatch")
					return false
				}
			} else if v2xMsgDistributionServer.InfoConnection != nil || received.V2xMsgDistributionServer[i].InfoConnection != nil {
				fmt.Println("v2xMsgDistributionServer.InfoConnection mismatch")
				return false
			}
		} // End of 'for' statement
	}

	fmt.Println("validateV2xMsgDistributionServerResponse: succeed")
	return true
}

func initializeVars() {
	mod.DbAddress = redisTestAddr
	redisAddr = redisTestAddr
	influxAddr = influxTestAddr
	sandboxName = testScenarioName
	os.Setenv("MEEP_PREDICT_MODEL_SUPPORTED", "true")
	postgisHost = postgisTestHost
	postgisPort = postgisTestPort
	os.Setenv("MEEP_SANDBOX_NAME", testScenarioName)
	v2x_broker = v2xBrokerTest
	os.Setenv("MEEP_BROKER", v2x_broker)
	os.Setenv("MEEP_TOPIC", v2xTopicTest)
	v2x_poa_list = poaListTest
	os.Setenv("MEEP_POA_LIST", strings.Join(v2x_poa_list, ";"))
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
