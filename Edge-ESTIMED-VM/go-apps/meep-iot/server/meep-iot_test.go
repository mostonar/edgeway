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

func TestRegisterediotplatformsPOST(t *testing.T) {
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
	var expected_adresses = []Addresses{}
	expected_adresses = append(expected_adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var expected_endpoint = EndPointInfo{
		Addresses: expected_adresses,
	}
	var expected_userTransportInfo = []MbTransportInfo{}
	//var mb_transportInfo TransportType
	mb_transportInfo := MB_TOPIC_BASED
	var security = SecurityInfo{}
	var implSpecificInfo = ImplSpecificInfo{}
	expected_userTransportInfo = append(expected_userTransportInfo, MbTransportInfo{
		Id:               "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:             "MQTT",
		Type_:            &mb_transportInfo,
		Description:      "MQTT",
		Protocol:         "MQTT",
		Version:          "2",
		Endpoint:         &expected_endpoint,
		Security:         &security,
		ImplSpecificInfo: &implSpecificInfo,
	})
	var expected_adresses_1 = []Addresses{}
	expected_adresses_1 = append(expected_adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var expected_customServicesTransportInfo = []TransportInfo{}
	var expected_endPointInfo_1 = EndPointInfo{
		Addresses: expected_adresses_1,
	}
	//var transportInfo TransportType
	transportInfo := REST_HTTP
	expected_customServicesTransportInfo = append(expected_customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "/laboai-acme-ic-cse",
		Type_:       &transportInfo,
		Description: "ACME oneM2M CSE",
		Protocol:    "REST_HTTP",
		Version:     "4",
		Endpoint:    &expected_endPointInfo_1,
		Security:    &security,
	})
	var expected_iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           expected_userTransportInfo,
		CustomServicesTransportInfo: expected_customServicesTransportInfo,
		Enabled:                     true,
	}
	fmt.Println("Create an IotPlatformInfo: ", expected_iotPlatformInfo)
	expected_iotPlatformInfo_str, err := json.Marshal(expected_iotPlatformInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_iotPlatformInfo: ", string(expected_iotPlatformInfo_str))

	/******************************
	 * request body section
	 ******************************/
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{
		Id:               "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:             "MQTT",
		Description:      "MQTT",
		Type_:            &mb_transportInfo,
		Protocol:         "MQTT",
		Version:          "2",
		Endpoint:         &endpoint,
		Security:         &security,
		ImplSpecificInfo: &implSpecificInfo,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "/laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Type_:       &transportInfo,
		Protocol:    "REST_HTTP",
		Version:     "4",
		Endpoint:    &endPointInfo_1,
		Security:    &security,
	})
	var iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	fmt.Println("Create an IotPlatformInfo: ", iotPlatformInfo)
	body, err := json.Marshal(iotPlatformInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/registered_iot_platforms", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, RegisterediotplatformsPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp IotPlatformInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validate_iot_platform_info(resp, iotPlatformInfo) {
		t.Errorf("handler returned unexpected body: got %v want %v", rr, expected_iotPlatformInfo_str)
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_registered_iot_platforms(resp.IotPlatformId)
	terminateScenario()
}

func TestRegisterediotplatformsPOSTFail_1(t *testing.T) {
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
	 * request body section
	 ******************************/
	//var mb_transportInfo TransportType
	mb_transportInfo := MB_TOPIC_BASED
	//var transportInfo TransportType
	transportInfo := REST_HTTP
	//var security = SecurityInfo{} // Missing Security
	var implSpecificInfo = ImplSpecificInfo{}
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{ // Missing Security
		Id:               "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:             "MQTT",
		Description:      "MQTT",
		Type_:            &mb_transportInfo,
		Protocol:         "MQTT",
		Version:          "2",
		Endpoint:         &endpoint,
		ImplSpecificInfo: &implSpecificInfo,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "/laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Type_:       &transportInfo,
		Protocol:    "REST_HTTP",
		Version:     "4",
		Endpoint:    &endPointInfo_1,
	})
	var iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	fmt.Println("Create an IotPlatformInfo: ", iotPlatformInfo)
	body, err := json.Marshal(iotPlatformInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/registered_iot_platforms", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, RegisterediotplatformsPOST)
	if err != nil {
		t.Fatalf("Shall received 400 Bad Request due to Security missing field")
	}
	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestRegisterediotplatformsPOSTFail_2(t *testing.T) {
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
	 * request body section
	 ******************************/
	//var mb_transportInfo TransportType
	mb_transportInfo := MB_TOPIC_BASED
	//var transportInfo TransportType
	transportInfo := REST_HTTP
	var security = SecurityInfo{}
	//var implSpecificInfo = ImplSpecificInfo{} // Missing ImplSpecificInfo
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{ // Missing ImplSpecificInfo
		Id:          "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:        "MQTT",
		Description: "MQTT",
		Type_:       &mb_transportInfo,
		Protocol:    "MQTT",
		Version:     "2",
		Endpoint:    &endpoint,
		Security:    &security,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "/laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Type_:       &transportInfo,
		Protocol:    "REST_HTTP",
		Version:     "4",
		Endpoint:    &endPointInfo_1,
		Security:    &security,
	})
	var iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	fmt.Println("Create an IotPlatformInfo: ", iotPlatformInfo)
	body, err := json.Marshal(iotPlatformInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/registered_iot_platforms", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, RegisterediotplatformsPOST)
	if err != nil {
		t.Fatalf("Shall received 400 Bad Request due to ImplSpecificInfo missing field")
	}
	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestRegisterediotplatformsPOSTFail_3(t *testing.T) {
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
	 * request body section
	 ******************************/
	//mb_transportInfo := MB_TOPIC_BASED // Missing TransportType
	transportInfo := REST_HTTP
	var security = SecurityInfo{}
	var implSpecificInfo = ImplSpecificInfo{}
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{ // Missing TransportType
		Id:               "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:             "MQTT",
		Description:      "MQTT",
		Protocol:         "MQTT",
		Version:          "2",
		Endpoint:         &endpoint,
		Security:         &security,
		ImplSpecificInfo: &implSpecificInfo,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "/laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Type_:       &transportInfo,
		Protocol:    "REST_HTTP",
		Version:     "4",
		Endpoint:    &endPointInfo_1,
		Security:    &security,
	})
	var iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	fmt.Println("Create an IotPlatformInfo: ", iotPlatformInfo)
	body, err := json.Marshal(iotPlatformInfo)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))
	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodPost, "/registered_iot_platforms", bytes.NewBuffer(body), nil, nil, nil, http.StatusBadRequest, RegisterediotplatformsPOST)
	if err != nil {
		t.Fatalf("Shall received 400 Bad Request due to TransportType missing field")
	}
	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestRegisterediotplatformsGET(t *testing.T) {
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
	expected_iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("expected_iotPlatformInfo: ", expected_iotPlatformInfo)

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/registered_iot_platforms", nil, nil, nil, nil, http.StatusOK, RegisterediotplatformsGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp []IotPlatformInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if len(resp) != 1 {
		t.Fatalf("Invalid response length")
	}
	if !validate_iot_platform_info(resp[0], expected_iotPlatformInfo) {
		t.Errorf("Response mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_registered_iot_platforms(expected_iotPlatformInfo.IotPlatformId)
	terminateScenario()
}

func TestRegisterediotplatformsByIdGET(t *testing.T) {
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
	expected_iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("expected_iotPlatformInfo: ", expected_iotPlatformInfo)

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["registeredIotPlatformId"] = expected_iotPlatformInfo.IotPlatformId

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/registered_iot_platforms", nil, vars, nil, nil, http.StatusOK, RegisterediotplatformsByIdGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp IotPlatformInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validate_iot_platform_info(resp, expected_iotPlatformInfo) {
		t.Errorf("Response mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_registered_iot_platforms(expected_iotPlatformInfo.IotPlatformId)
	terminateScenario()
}

func TestRegisterediotplatformsDelete(t *testing.T) {
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
	expected_iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("expected_iotPlatformInfo: ", expected_iotPlatformInfo)

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["registeredIotPlatformId"] = expected_iotPlatformInfo.IotPlatformId

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodDelete, "/registered_iot_platforms", nil, vars, nil, nil, http.StatusNoContent, RegisterediotplatformsByIdDELETE)
	if err != nil {
		t.Fatalf(err.Error())
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	terminateScenario()
}

func TestRegistereddevicesPOST(t *testing.T) {
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

	iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("iotPlatformInfo: ", iotPlatformInfo)

	/******************************
	 * expected response section
	 ******************************/
	expected_requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	expected_deviceMetadata := make([]KeyValuePair, 2)
	expected_deviceMetadata[0] = KeyValuePair{
		Key:   "Key1",
		Value: "Value1",
	}
	expected_deviceMetadata[1] = KeyValuePair{
		Key:   "Key2",
		Value: "Value2",
	}
	var expected_device = DeviceInfo{
		DeviceMetadata:         expected_deviceMetadata,
		RequestedIotPlatformId: expected_requestedIotPlatformId,
		DeviceId:               expected_requestedIotPlatformId,
		Imsi:                   "12345",
		Enabled:                true,
	}
	expected_device_str, err := json.Marshal(expected_device)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("expected_device: ", string(expected_device_str))

	/******************************
	 * request body section
	 ******************************/
	requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	deviceMetadata := make([]KeyValuePair, 2)
	deviceMetadata[0] = KeyValuePair{
		Key:   "Key1",
		Value: "Value1",
	}
	deviceMetadata[1] = KeyValuePair{
		Key:   "Key2",
		Value: "Value2",
	}
	var device = DeviceInfo{
		DeviceMetadata:         deviceMetadata,
		RequestedIotPlatformId: requestedIotPlatformId,
		DeviceId:               requestedIotPlatformId,
		Imsi:                   "12345",
		Enabled:                true,
	}
	fmt.Println("Create a device: ", device)
	body, err := json.Marshal(device)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("body: ", string(body))

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/registered_devices", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, RegistereddevicesPOST)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp DeviceInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validate_device_info(resp, expected_device) {
		t.Errorf("Response mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_device(expected_device.DeviceId)
	_ = delete_registered_iot_platforms(iotPlatformInfo.IotPlatformId)
	terminateScenario()
}

func TestRegistereddevicesDELETE(t *testing.T) {
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

	iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("iotPlatformInfo: ", iotPlatformInfo)

	/******************************
	 * expected response section
	 ******************************/
	device, err := create_device(iotPlatformInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["registeredDeviceId"] = device.DeviceId

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	_, err = sendRequest(http.MethodDelete, "/registered_devices", nil, vars, nil, nil, http.StatusNoContent, RegistereddevicesByIdDELETE)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_registered_iot_platforms(iotPlatformInfo.IotPlatformId)
	terminateScenario()
}

func TestRegistereddevicesGET(t *testing.T) {
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

	iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("iotPlatformInfo: ", iotPlatformInfo)

	/******************************
	 * expected response section
	 ******************************/
	expected_device, err := create_device(iotPlatformInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/registered_devices", nil, nil, nil, nil, http.StatusOK, RegistereddevicesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp []DeviceInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if len(resp) != 1 {
		t.Errorf("Response length mismatch")
	}
	if !validate_device_info(resp[0], expected_device) {
		t.Errorf("Response mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_device(expected_device.DeviceId)
	_ = delete_registered_iot_platforms(iotPlatformInfo.IotPlatformId)
	terminateScenario()
}

func TestRegistereddevicesGET_withFiltering(t *testing.T) {
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

	iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("iotPlatformInfo: ", iotPlatformInfo)

	/******************************
	 * expected response section
	 ******************************/
	expected_devices, err := create_set_of_devices(iotPlatformInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Filter on DeviceId and its complement
	queryParam := make(map[string]string)
	queryParam["filter"] = "(eq,deviceId," + expected_devices[1].DeviceId + ")" // Only DeviceId 1
	rr, err := sendRequest(http.MethodGet, "/registered_devices", nil, nil, queryParam, nil, http.StatusOK, RegistereddevicesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp []DeviceInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if len(resp) != 1 { // Only DeviceId 1
		t.Errorf("Response length mismatch")
	}
	if !validate_device_info(resp[0], expected_devices[1]) {
		t.Errorf("Response mismatch")
	}
	queryParam["filter"] = "(neq,deviceId," + expected_devices[1].DeviceId + ")" // DeviceId 1 is excluded
	rr, err = sendRequest(http.MethodGet, "/registered_devices", nil, nil, queryParam, nil, http.StatusOK, RegistereddevicesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	resp = nil
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if len(resp) != 2 {
		t.Errorf("Response length mismatch")
	}
	if !validate_device_info(resp[0], expected_devices[0]) && !validate_device_info(resp[0], expected_devices[2]) { // DeviceId 1 is excluded
		t.Errorf("Response mismatch")
	}
	if !validate_device_info(resp[1], expected_devices[0]) && !validate_device_info(resp[1], expected_devices[2]) { // DeviceId 1 is excluded
		t.Errorf("Response mismatch")
	}

	// Filter on RequestedUserTransportId and its complement
	queryParam["filter"] = "(eq,requestedUserTransportId," + expected_devices[2].RequestedUserTransportId + ")" // Only DeviceId 2
	rr, err = sendRequest(http.MethodGet, "/registered_devices", nil, nil, queryParam, nil, http.StatusOK, RegistereddevicesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	resp = nil
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if len(resp) != 1 { // Only DeviceId 2
		t.Errorf("Response length mismatch")
	}
	if !validate_device_info(resp[0], expected_devices[2]) {
		t.Errorf("Response mismatch")
	}
	queryParam["filter"] = "(neq,requestedUserTransportId," + expected_devices[2].RequestedUserTransportId + ")" // DeviceId 2 is excluded
	rr, err = sendRequest(http.MethodGet, "/registered_devices", nil, nil, queryParam, nil, http.StatusOK, RegistereddevicesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	resp = nil
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if len(resp) != 2 {
		t.Errorf("Response length mismatch")
	}
	if !validate_device_info(resp[0], expected_devices[0]) && !validate_device_info(resp[0], expected_devices[1]) { // DeviceId 2 is excluded
		t.Errorf("Response mismatch")
	}
	if !validate_device_info(resp[1], expected_devices[0]) && !validate_device_info(resp[1], expected_devices[1]) { // DeviceId 2 is excluded
		t.Errorf("Response mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_registered_iot_platforms(iotPlatformInfo.IotPlatformId)
	terminateScenario()
}

func TestRegistereddevicesByIdGET(t *testing.T) {
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

	iotPlatformInfo, err := create_registered_iot_platforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fmt.Println("iotPlatformInfo: ", iotPlatformInfo)

	/******************************
	 * expected response section
	 ******************************/
	expected_device, err := create_device(iotPlatformInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	/******************************
	 * request vars section
	 ******************************/
	vars := make(map[string]string)
	vars["registeredDeviceId"] = expected_device.DeviceId

	/******************************
	 * request body section
	 ******************************/

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodGet, "/registered_devices", nil, vars, nil, nil, http.StatusOK, RegistereddevicesByIdGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: rr: ", rr)
	var resp DeviceInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Respone: resp: ", resp)
	if !validate_device_info(resp, expected_device) {
		t.Errorf("Response mismatch")
	}

	fmt.Println("Received expected response")
	/******************************
	 * back to initial state section
	 ******************************/
	_ = delete_device(expected_device.DeviceId)
	_ = delete_registered_iot_platforms(iotPlatformInfo.IotPlatformId)
	terminateScenario()
}

func create_registered_iot_platforms() (resp IotPlatformInfo, err error) {
	mb_transportInfo := MB_TOPIC_BASED
	var security = SecurityInfo{}
	var implSpecificInfo = ImplSpecificInfo{}
	transportInfo := REST_HTTP
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{
		Id:               "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:             "MQTT",
		Description:      "MQTT",
		Type_:            &mb_transportInfo,
		Protocol:         "MQTT",
		Version:          "2",
		Endpoint:         &endpoint,
		Security:         &security,
		ImplSpecificInfo: &implSpecificInfo,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "/laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Type_:       &transportInfo,
		Protocol:    "REST_HTTP",
		Version:     "4",
		Endpoint:    &endPointInfo_1,
		Security:    &security,
	})
	var iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	fmt.Println("Create an IotPlatformInfo: ", iotPlatformInfo)
	body, err := json.Marshal(iotPlatformInfo)
	if err != nil {
		return resp, err
	}
	fmt.Println("body: ", string(body))

	rr, err := sendRequest(http.MethodPost, "/registered_iot_platforms", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, RegisterediotplatformsPOST)
	if err != nil {
		return resp, err
	}
	fmt.Println("Respone: rr: ", rr)
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func delete_registered_iot_platforms(iotPlatformId string) (err error) {
	vars := make(map[string]string)
	vars["registeredIotPlatformId"] = iotPlatformId
	_, err = sendRequest(http.MethodDelete, "/registered_iot_platforms", nil, vars, nil, nil, http.StatusNoContent, RegisterediotplatformsByIdDELETE)
	if err != nil {
		return err
	}

	return nil
}

func validate_iot_platform_info(received IotPlatformInfo, expected IotPlatformInfo) bool {
	fmt.Println("validate_iot_platform_info: received: ", received)
	fmt.Println("validate_iot_platform_info: expected: ", expected)

	if received.IotPlatformId != expected.IotPlatformId {
		fmt.Println("received.IotPlatformId != IotPlatformId")
		return false
	}

	if len(received.UserTransportInfo) != len(expected.UserTransportInfo) {
		fmt.Println("len(received.UserTransportInfo) mismatch")
		return false
		// TODO FSCOM } else {
	}

	if len(received.CustomServicesTransportInfo) != len(expected.CustomServicesTransportInfo) {
		fmt.Println("len(received.CustomServicesTransportInfo) mismatch")
		return false
		// TODO FSCOM } else {
	}

	if received.Enabled != expected.Enabled {
		fmt.Println("received.Enabled != Enabled")
		return false
	}

	fmt.Println("validate_iot_platform_info: succeed")
	return true
}

func create_device(iotPlatformInfo IotPlatformInfo) (device DeviceInfo, err error) {
	requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	deviceMetadata := make([]KeyValuePair, 2)
	deviceMetadata[0] = KeyValuePair{
		Key:   "Key1",
		Value: "Value1",
	}
	deviceMetadata[1] = KeyValuePair{
		Key:   "Key2",
		Value: "Value2",
	}
	device = DeviceInfo{
		DeviceMetadata:         deviceMetadata,
		RequestedIotPlatformId: requestedIotPlatformId,
		DeviceId:               requestedIotPlatformId,
		Imsi:                   "12345",
		Enabled:                true,
	}
	body, err := json.Marshal(device)
	if err != nil {
		return device, err
	}

	/******************************
	 * request execution section
	 ******************************/
	rr, err := sendRequest(http.MethodPost, "/registered_devices", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, RegistereddevicesPOST)
	if err != nil {
		return device, err
	}
	fmt.Println("Respone: rr: ", rr)
	var resp DeviceInfo
	err = json.Unmarshal([]byte(rr), &resp)
	if err != nil {
		return device, err
	}

	return device, nil
}

func create_set_of_devices(iotPlatformInfo IotPlatformInfo) (devices []DeviceInfo, err error) {
	requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	deviceMetadata := make([]KeyValuePair, 2)
	deviceMetadata[0] = KeyValuePair{
		Key:   "Key1",
		Value: "Value1",
	}
	deviceMetadata[1] = KeyValuePair{
		Key:   "Key2",
		Value: "Value2",
	}
	devices = make([]DeviceInfo, 3)
	devices[0] = DeviceInfo{
		DeviceMetadata:         deviceMetadata,
		RequestedIotPlatformId: requestedIotPlatformId,
		DeviceId:               "0",
		Gpsi:                   "12345",
		Enabled:                true,
	}
	deviceMetadata[0].Key = "Key11"
	devices[1] = DeviceInfo{
		DeviceMetadata:         deviceMetadata,
		RequestedIotPlatformId: requestedIotPlatformId,
		DeviceId:               "1",
		Msisdn:                 "345",
		Enabled:                true,
	}
	deviceMetadata[0].Key = "Key12"
	devices[2] = DeviceInfo{
		DeviceMetadata:           deviceMetadata,
		RequestedIotPlatformId:   requestedIotPlatformId,
		DeviceId:                 "2",
		RequestedUserTransportId: "987",
		Imsi:                     "12345",
		Enabled:                  true,
	}

	/******************************
	 * request execution section
	 ******************************/
	resp_devices := devices // Same format
	for i, val := range devices {
		body, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		rr, err := sendRequest(http.MethodPost, "/registered_devices", bytes.NewBuffer(body), nil, nil, nil, http.StatusCreated, RegistereddevicesPOST)
		if err != nil {
			return nil, err
		}
		fmt.Println("Respone: rr: ", rr)
		var resp DeviceInfo
		err = json.Unmarshal([]byte(rr), &resp)
		if err != nil {
			return nil, err
		}
		resp_devices[i] = resp
	} // End of 'for' statement

	return resp_devices, nil
}

func delete_device(deviceId string) (err error) {
	vars := make(map[string]string)
	vars["registeredDeviceId"] = deviceId
	_, err = sendRequest(http.MethodDelete, "/registered_devices", nil, vars, nil, nil, http.StatusNoContent, RegistereddevicesByIdDELETE)
	return err
}

func validate_device_info(expected_device DeviceInfo, received_deviceResp DeviceInfo) bool {
	fmt.Println(">>> validate_device_info: expected_device: ", expected_device)
	fmt.Println(">>> validate_device_info: received_deviceResp: ", received_deviceResp)

	if expected_device.DeviceId != received_deviceResp.DeviceId {
		fmt.Println("received_deviceResp.DeviceId != DeviceId")
		return false
	}
	if expected_device.Enabled != received_deviceResp.Enabled {
		fmt.Println("received_deviceResp.Enabled != Enabled")
		return false
	}
	if expected_device.DeviceAuthenticationInfo != received_deviceResp.DeviceAuthenticationInfo {
		fmt.Println("received_deviceResp.DeviceAuthenticationInfo != DeviceAuthenticationInfo")
		return false
	}
	if expected_device.Gpsi != received_deviceResp.Gpsi {
		fmt.Println("received_deviceResp.Gpsi != Gpsi")
		return false
	}
	if expected_device.Pei != received_deviceResp.Pei {
		fmt.Println("received_deviceResp.Pei != Pei")
		return false
	}
	if expected_device.Supi != received_deviceResp.Supi {
		fmt.Println("received_deviceResp.Supi != Supi")
		return false
	}
	if expected_device.Msisdn != received_deviceResp.Msisdn {
		fmt.Println("received_deviceResp.Msisdn != Msisdn")
		return false
	}
	if expected_device.Imei != received_deviceResp.Imei {
		fmt.Println("received_deviceResp.Imei != Imei")
		return false
	}
	if expected_device.Imsi != received_deviceResp.Imsi {
		fmt.Println("received_deviceResp.Imsi != Imsi")
		return false
	}
	if expected_device.Iccid != received_deviceResp.Iccid {
		fmt.Println("received_deviceResp.Iccid != Iccid")
		return false
	}
	if expected_device.RequestedIotPlatformId != received_deviceResp.RequestedIotPlatformId {
		fmt.Println("received_deviceResp.RequestedIotPlatformId != RequestedIotPlatformId")
		return false
	}
	if expected_device.RequestedUserTransportId != received_deviceResp.RequestedUserTransportId {
		fmt.Println("received_deviceResp.RequestedUserTransportId != RequestedUserTransportId")
		return false
	}
	if expected_device.ClientCertificate != received_deviceResp.ClientCertificate {
		fmt.Println("received_deviceResp.ClientCertificate != ClientCertificate")
		return false
	}
	if len(expected_device.DeviceMetadata) != len(received_deviceResp.DeviceMetadata) {
		fmt.Println("received_deviceResp.DeviceMetadata != DeviceMetadata")
		return false
	} else {
		for i, val := range expected_device.DeviceMetadata {
			if val.Key != received_deviceResp.DeviceMetadata[i].Key {
				fmt.Println("item #", i, ":received_deviceResp.DeviceMetadata.Key != DeviceMetadata.Key")
				return false
			}
			if val.Value != received_deviceResp.DeviceMetadata[i].Value {
				fmt.Println("item #", i, ":received_deviceResp.DeviceMetadata.Value != DeviceMetadata.Value")
				return false

			}
		} // End of 'for' statement
	}
	if len(expected_device.RequestedMecTrafficRule) != len(received_deviceResp.RequestedMecTrafficRule) {
		fmt.Println("received_deviceResp.RequestedMecTrafficRule != RequestedMecTrafficRule")
		return false
		// TODO FSCOM } else {
		// for i, val := range expected_device.RequestedMecTrafficRule {
		// } // End of 'for' statement
		// TODO FSCOM To be continued
		//RequestedMecTrafficRule  []TrafficRuleDescriptor
	}
	// TODO FSCOM To be continued

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
			s := fmt.Sprintf("Header Location expected")
			return "", errors.New(s)
		} else if !strings.Contains(s, *location) {
			s := fmt.Sprintf("Wrong Header Location - got %s want %s", s, *location)
			return "", errors.New(s)
		}
	}

	return string(rr.Body.String()), nil
}
