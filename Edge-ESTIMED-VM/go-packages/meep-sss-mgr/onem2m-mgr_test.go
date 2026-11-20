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

package sssmgr

import (
	"fmt"
	"reflect"
	"testing"
	//"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

const tmName = "meep-iot"
const tmNamespace = "sandboxtest"

// func TestNewSssMgr(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Invalid Connector
// 	fmt.Println("Invalid SSS Asset Manager")
// 	tm, err := NewSssMgr("", tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err == nil || tm != nil {
// 		t.Fatalf("Service name not set")
// 	}
// 	tm, err = NewSssMgr(tmName, tmNamespace, "", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err == nil || tm != nil {
// 		t.Fatalf("Binding protocol not set")
// 	}
// 	tm, err = NewSssMgr(tmName, tmNamespace, "MQTT", "", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err == nil || tm != nil {
// 		t.Fatalf("Host not set")
// 	}
// 	tm, err = NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err == nil || tm != nil {
// 		t.Fatalf("Host id not set")
// 	}
// 	tm, err = NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "", nil, nil, nil)
// 	if err == nil || tm != nil {
// 		t.Fatalf("CSE name not set")
// 	}

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err = NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}
// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil

// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err = NewSssMgr(tmName, tmNamespace, "REST_HTTP", "172.29.10.56", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestPopulateDevicesPerIotPlatformsHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())
// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	err = tm.populateDevicesPerIotPlatforms()
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestSensorDiscoveryInfoAllHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())
// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "", 0, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	sensors, err := tm.SensorDiscoveryInfoAll()
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("len=", len(sensors))
// 	fmt.Println("sensors", sensors)

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestGetSensorHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	sensors, err := tm.SensorDiscoveryInfoAll()
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}

// 	for _, v := range sensors {
// 		fmt.Println("v", v)
// 		fmt.Println("TypeOf(v)", reflect.TypeOf(v))

// 		sensor, err := tm.GetSensor(v.SensorIdentifier)
// 		if !validate_sensor_discovery_info(v, sensor) {
// 			t.Fatalf(err.Error())
// 		}
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_createAEHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var sensor = SensorDiscoveryInfo{
// 		SensorIdentifier: "12345",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	new_sensor, err := tm.OneM2M_create(sensor, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_ae(sensor, new_sensor) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	_ = tm.OneM2M_delete(new_sensor)

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_createAE_CNTHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var sensor_ae = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyAE",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	new_sensor_ae, err := tm.OneM2M_create(sensor_ae, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new AE sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_ae(sensor_ae, new_sensor_ae) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	var sensor_cnt = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyCNT",
// 		SensorType:       "CNT",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	// sensor_cnt.SensorCharacteristicList = make([]SensorCharacteristic, 1)
// 	// sensor_cnt.SensorCharacteristicList[0] = SensorCharacteristic{CharacteristicName: "con", CharacteristicValue: "OFF"}
// 	sensorPath := new_sensor_ae.SensorIdentifier
// 	new_sensor_cnt, err := tm.OneM2M_create(sensor_cnt, sensorPath)
// 	if err != nil {
// 		t.Fatalf("Failed to create new CNT sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_cnt(sensor_cnt, new_sensor_cnt) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	err = tm.OneM2M_delete(new_sensor_cnt)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}
// 	err = tm.OneM2M_delete(new_sensor_ae)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

func TestOneM2M_createAE_FLEXHttp(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid SSS Asset Manager")
	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create SSS Asset Manager")
	}

	var sensor_ae = SensorDiscoveryInfo{
		SensorIdentifier: "CMyAE",
		SensorType:       "AE",
		SensorPosition:   nil,
		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
	}
	new_sensor_ae, err := tm.OneM2M_create(sensor_ae, "", "CMyAE")
	if err != nil {
		t.Fatalf("Failed to create new AE sensor")
	}

	// Verify content
	if !validate_sensor_ae(sensor_ae, new_sensor_ae) {
		t.Fatalf("Failed to validate AE content")
	}

	var sensor_flex = SensorDiscoveryInfo{
		SensorIdentifier: "CMyFLX",
		SensorType:       "FLX",
		SensorPosition:   nil,
		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
	}
	sensor_flex.Flex = make(map[string]interface{}, 0)
	sensor_flex.Flex["type"] = "cod:color"
	sensor_flex.Flex["cnd"] = "org.onem2m.common.moduleclass.colour"
	sensor_flex.Flex["red"] = 20
	sensor_flex.Flex["green"] = 20
	sensor_flex.Flex["blue"] = 20
	sensorPath := new_sensor_ae.SensorIdentifier
	new_sensor_flex, err := tm.OneM2M_create(sensor_flex, sensorPath, "CMyAE")
	if err != nil {
		t.Fatalf("Failed to create new CNT sensor")
	}

	// Verify content
	if !validate_sensor_flex(sensor_flex, new_sensor_flex) {
		t.Fatalf("Failed to validate FLEX content")
	}

	sensors, err := tm.SensorDiscoveryInfoAll()
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("len=", len(sensors))
	fmt.Println("sensors", sensors)

	err = tm.OneM2M_delete(new_sensor_flex, "CMyAE")
	if err != nil {
		t.Fatalf("Failed to create new sensor")
	}
	err = tm.OneM2M_delete(new_sensor_ae, "CMyAE")
	if err != nil {
		t.Fatalf("Failed to create new sensor")
	}

	// Cleanup
	err = tm.DeleteSssMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup SSS Asset Manager")
	}
	tm = nil
}

// func TestOneM2M_createAE_CNT_CNIHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var sensor_ae = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyAE",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	new_sensor_ae, err := tm.OneM2M_create(sensor_ae, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new AE sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_ae(sensor_ae, new_sensor_ae) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	var sensor_cnt = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyCNT",
// 		SensorType:       "CNT",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	sensorPath := new_sensor_ae.SensorIdentifier
// 	new_sensor_cnt, err := tm.OneM2M_create(sensor_cnt, sensorPath)
// 	if err != nil {
// 		t.Fatalf("Failed to create new CNT sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_cnt(sensor_cnt, new_sensor_cnt) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	var sensor_cin = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyCNI",
// 		SensorType:       "CIN",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	sensor_cin.SensorCharacteristicList = make([]SensorCharacteristic, 1)
// 	sensor_cin.SensorCharacteristicList[0] = SensorCharacteristic{CharacteristicName: "con", CharacteristicValue: "OFF"}
// 	sensorPath = new_sensor_cnt.SensorIdentifier
// 	new_sensor_cin, err := tm.OneM2M_create(sensor_cin, sensorPath)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_cin(sensor_cin, new_sensor_cin) {
// 		t.Fatalf("Failed to validate CIN content")
// 	}

// 	err = tm.OneM2M_delete(new_sensor_cin)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}
// 	err = tm.OneM2M_delete(new_sensor_cnt)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}
// 	err = tm.OneM2M_delete(new_sensor_ae)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_deleteHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var new_sensor = SensorDiscoveryInfo{
// 		SensorIdentifier: "12345",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	sensor, err := oneM2M_create(tm, new_sensor, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor: " + err.Error())
// 	}

// 	err = tm.OneM2M_delete(sensor)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor: " + err.Error())
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_getHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	sensors, received_sensors, err := oneM2M_createAE_CNT_CIN(tm)
// 	if err != nil {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Verify content
// 	if len(sensors) != 3 || len(received_sensors) != 3 {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Get one sensor
// 	sensorResp, err := tm.OneM2M_get(received_sensors["CNT"].SensorIdentifier, received_sensors["CNT"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover CNT sensor")
// 	}
// 	// Verify content
// 	if !validate_sensor_cnt(received_sensors["CNT"], sensorResp) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	sensorResp, err = tm.OneM2M_get(received_sensors["CIN"].SensorIdentifier, received_sensors["CIN"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover CNT sensor")
// 	}
// 	// Verify content
// 	if !validate_sensor_cin(received_sensors["CIN"], sensorResp) {
// 		t.Fatalf("Failed to validate CIN content")
// 	}

// 	_ = tm.OneM2M_delete(received_sensors["CIN"])
// 	_ = tm.OneM2M_delete(received_sensors["CNT"])
// 	_ = tm.OneM2M_delete(received_sensors["AE"])

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_discoveryHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	_, received_sensors, err := oneM2M_createAE_CNT_CIN(tm)
// 	if err != nil {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Verify content
// 	if len(received_sensors) != 3 {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Discover CNTs
// 	sensorList, err := tm.OneM2M_discovery(received_sensors["CNT"].SensorType, received_sensors["CNT"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover CNT sensor")
// 	}
// 	if len(sensorList) != 1 {
// 		t.Fatalf("Wrong discover result")
// 	}
// 	// Verify content
// 	if !validate_sensor_cnt(received_sensors["CNT"], sensorList[0]) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	// Discover AEs
// 	sensorList, err = tm.OneM2M_discovery(received_sensors["AE"].SensorType, received_sensors["AE"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover AE sensor")
// 	}
// 	if len(sensorList) != 1 {
// 		t.Fatalf("Wrong discover result")
// 	}
// 	// Verify content
// 	if !validate_sensor_ae(received_sensors["AE"], sensorList[0]) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	_ = tm.OneM2M_delete(received_sensors["CIN"])
// 	_ = tm.OneM2M_delete(received_sensors["CNT"])
// 	_ = tm.OneM2M_delete(received_sensors["AE"])

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_subscribeHttp(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "lab-oai.etsi.org", 31110, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", discoveryNotify, statusNotify, dataNotify)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	_, received_sensors, err := oneM2M_createAE_CNT_CIN(tm)
// 	if err != nil {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	subscriptionMap := make(map[string]interface{})
// 	subId, err := tm.OneM2M_subscribe(received_sensors["AE"].IotPlatformId, received_sensors["AE"].SensorIdentifier)
// 	if err != nil {
// 		t.Fatalf("Failed to subscribe")
// 	}
// 	fmt.Println("subId=" + subId)
// 	subscriptionMap[subId] = received_sensors["AE"]

// 	subId, err = tm.OneM2M_subscribe(received_sensors["CNT"].IotPlatformId, received_sensors["CNT"].SensorIdentifier)
// 	if err != nil {
// 		t.Fatalf("Failed to subscribe")
// 	}
// 	fmt.Println("subId=" + subId)
// 	subscriptionMap[subId] = received_sensors["CNT"]

// 	fmt.Println("len(subscriptionMap)=" + fmt.Sprint(len(subscriptionMap)))

// 	fmt.Println("You have 120 seconds to trigger subscriptions")
// 	time.Sleep(time.Duration(120) * time.Second)

// 	for k := range subscriptionMap {
// 		err = tm.OneM2M_delete_subscription(k)
// 		if err != nil {
// 			t.Fatalf("Failed to cancel subscription")
// 		}
// 	} // End of 'for' statement

// 	_ = tm.OneM2M_delete(received_sensors["CIN"])
// 	_ = tm.OneM2M_delete(received_sensors["CNT"])
// 	_ = tm.OneM2M_delete(received_sensors["AE"])

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestPopulateDevicesPerIotPlatformsMqtt(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())
// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}
// 	err = tm.populateDevicesPerIotPlatforms()
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestSensorDiscoveryInfoAllMqtt(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	err = tm.populateDevicesPerIotPlatforms()
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}

// 	sensors, err := tm.SensorDiscoveryInfoAll()
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	fmt.Println("sensors: ", sensors)

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// }

// func TestGetSensorMqtt(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	sensors, err := tm.SensorDiscoveryInfoAll()
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}

// 	for _, v := range sensors {
// 		fmt.Println("v", v)
// 		fmt.Println("TypeOf(v)", reflect.TypeOf(v))

// 		sensor, err := tm.GetSensor(v.SensorIdentifier)
// 		if !validate_sensor_discovery_info(v, sensor) {
// 			t.Fatalf(err.Error())
// 		}
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_createAEMQTT(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var sensor = SensorDiscoveryInfo{
// 		SensorIdentifier: "12345",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	new_sensor, err := tm.OneM2M_create(sensor, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_ae(sensor, new_sensor) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	_ = tm.OneM2M_delete(new_sensor)

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_createAE_CNTMQTT(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var sensor_ae = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyAE",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	new_sensor_ae, err := tm.OneM2M_create(sensor_ae, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new AE sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_ae(sensor_ae, new_sensor_ae) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	var sensor_cnt = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyCNT",
// 		SensorType:       "CNT",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	// sensor_cnt.SensorCharacteristicList = make([]SensorCharacteristic, 1)
// 	// sensor_cnt.SensorCharacteristicList[0] = SensorCharacteristic{CharacteristicName: "con", CharacteristicValue: "OFF"}
// 	sensorPath := new_sensor_ae.SensorIdentifier
// 	new_sensor_cnt, err := tm.OneM2M_create(sensor_cnt, sensorPath)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_cnt(sensor_cnt, new_sensor_cnt) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	err = tm.OneM2M_delete(new_sensor_cnt)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}
// 	err = tm.OneM2M_delete(new_sensor_ae)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_createAE_CNT_CNIMQTT(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var sensor_ae = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyAE",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	new_sensor_ae, err := tm.OneM2M_create(sensor_ae, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new AE sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_ae(sensor_ae, new_sensor_ae) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	var sensor_cnt = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyCNT",
// 		SensorType:       "CNT",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	sensorPath := new_sensor_ae.SensorIdentifier
// 	new_sensor_cnt, err := tm.OneM2M_create(sensor_cnt, sensorPath)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Verify content
// 	if !validate_sensor_cnt(sensor_cnt, new_sensor_cnt) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	var sensor_cin = SensorDiscoveryInfo{
// 		SensorIdentifier: "CMyCNI",
// 		SensorType:       "CIN",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	sensor_cin.SensorCharacteristicList = make([]SensorCharacteristic, 1)
// 	sensor_cin.SensorCharacteristicList[0] = SensorCharacteristic{CharacteristicName: "con", CharacteristicValue: "OFF"}
// 	sensorPath = new_sensor_cnt.SensorIdentifier
// 	new_sensor_cin, err := tm.OneM2M_create(sensor_cin, sensorPath)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}
// 	if new_sensor_cin.SensorType != sensor_cin.SensorType {
// 		t.Fatalf("received_sensor.SensorType != SensorType")
// 	}
// 	if new_sensor_cin.IotPlatformId != sensor_cin.IotPlatformId {
// 		t.Fatalf("received_sensor.IotPlatformId != IotPlatformId")
// 	}
// 	if len(new_sensor_cin.SensorCharacteristicList) == 0 {
// 		t.Fatalf("received_sensor.SensorCharacteristicList shall not be empty")
// 	}

// 	err = tm.OneM2M_delete(new_sensor_cin)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}
// 	err = tm.OneM2M_delete(new_sensor_cnt)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}
// 	err = tm.OneM2M_delete(new_sensor_ae)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor")
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_deleteMQTT(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	var new_sensor = SensorDiscoveryInfo{
// 		SensorIdentifier: "12345",
// 		SensorType:       "AE",
// 		SensorPosition:   nil,
// 		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
// 	}
// 	sensor, err := oneM2M_create(tm, new_sensor, "")
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor: " + err.Error())
// 	}

// 	err = tm.OneM2M_delete(sensor)
// 	if err != nil {
// 		t.Fatalf("Failed to create new sensor: " + err.Error())
// 	}

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_getMQTT(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	sensors, received_sensors, err := oneM2M_createAE_CNT_CIN(tm)
// 	if err != nil {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Verify content
// 	if len(sensors) != 3 || len(received_sensors) != 3 {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Get one sensor
// 	sensorResp, err := tm.OneM2M_get(received_sensors["CNT"].SensorIdentifier, received_sensors["CNT"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover CNT sensor")
// 	}
// 	// Verify content
// 	if !validate_sensor_cnt(received_sensors["CNT"], sensorResp) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	sensorResp, err = tm.OneM2M_get(received_sensors["CIN"].SensorIdentifier, received_sensors["CIN"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover CNT sensor")
// 	}
// 	// Verify content
// 	if !validate_sensor_cin(received_sensors["CIN"], sensorResp) {
// 		t.Fatalf("Failed to validate CIN content")
// 	}

// 	_ = tm.OneM2M_delete(received_sensors["CIN"])
// 	_ = tm.OneM2M_delete(received_sensors["CNT"])
// 	_ = tm.OneM2M_delete(received_sensors["AE"])

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

// func TestOneM2M_discoveryMQTT(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", nil, nil, nil)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	_, received_sensors, err := oneM2M_createAE_CNT_CIN(tm)
// 	if err != nil {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Verify content
// 	if len(received_sensors) != 3 {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	// Discover CNTs
// 	sensorList, err := tm.OneM2M_discovery(received_sensors["CNT"].SensorType, received_sensors["CNT"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover CNT sensor")
// 	}
// 	if len(sensorList) != 1 {
// 		t.Fatalf("Wrong discover result")
// 	}
// 	// Verify content
// 	if !validate_sensor_cnt(received_sensors["CNT"], sensorList[0]) {
// 		t.Fatalf("Failed to validate CNT content")
// 	}

// 	// Discover AEs
// 	sensorList, err = tm.OneM2M_discovery(received_sensors["AE"].SensorType, received_sensors["AE"].IotPlatformId)
// 	if err != nil {
// 		t.Fatalf("Failed to discover AE sensor")
// 	}
// 	if len(sensorList) != 1 {
// 		t.Fatalf("Wrong discover result")
// 	}
// 	// Verify content
// 	if !validate_sensor_ae(received_sensors["AE"], sensorList[0]) {
// 		t.Fatalf("Failed to validate AE content")
// 	}

// 	_ = tm.OneM2M_delete(received_sensors["CIN"])
// 	_ = tm.OneM2M_delete(received_sensors["CNT"])
// 	_ = tm.OneM2M_delete(received_sensors["AE"])

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }
// func TestOneM2M_subscribeMQTT(t *testing.T) {
// 	fmt.Println("--- ", t.Name())
// 	log.MeepTextLogInit(t.Name())

// 	// Valid Connector
// 	fmt.Println("Create valid SSS Asset Manager")
// 	tm, err := NewSssMgr(tmName, tmNamespace, "MQTT", "172.29.10.56", 1883, "0a692154-0f93-11f0-b554-df2f6a756f6a", "laboai-acme-ic-cse", discoveryNotify, statusNotify, dataNotify)
// 	if err != nil || tm == nil {
// 		t.Fatalf("Failed to create SSS Asset Manager")
// 	}

// 	_, received_sensors, err := oneM2M_createAE_CNT_CIN(tm)
// 	if err != nil {
// 		t.Fatalf("Failed to create sensors")
// 	}

// 	subscriptionMap := make(map[string]interface{})
// 	subId, err := tm.OneM2M_subscribe(received_sensors["AE"].IotPlatformId, received_sensors["AE"].SensorIdentifier)
// 	if err != nil {
// 		t.Fatalf("Failed to subscribe")
// 	}
// 	fmt.Println("subId=" + subId)
// 	subscriptionMap[subId] = received_sensors["AE"]

// 	subId, err = tm.OneM2M_subscribe(received_sensors["CNT"].IotPlatformId, received_sensors["CNT"].SensorIdentifier)
// 	if err != nil {
// 		t.Fatalf("Failed to subscribe")
// 	}
// 	fmt.Println("subId=" + subId)
// 	subscriptionMap[subId] = received_sensors["CNT"]

// 	fmt.Println("len(subscriptionMap)=" + fmt.Sprint(len(subscriptionMap)))

// 	fmt.Println("You have 120 seconds to trigger subscriptions")
// 	time.Sleep(time.Duration(120) * time.Second)

// 	for k := range subscriptionMap {
// 		err = tm.OneM2M_delete_subscription(k)
// 		if err != nil {
// 			t.Fatalf("Failed to cancel subscription")
// 		}
// 	} // End of 'for' statement

// 	_ = tm.OneM2M_delete(received_sensors["CIN"])
// 	_ = tm.OneM2M_delete(received_sensors["CNT"])
// 	_ = tm.OneM2M_delete(received_sensors["AE"])

// 	// Cleanup
// 	err = tm.DeleteSssMgr()
// 	if err != nil {
// 		t.Fatalf("Failed to cleanup SSS Asset Manager")
// 	}
// 	tm = nil
// }

func oneM2M_create(tm *SssMgr, sensor SensorDiscoveryInfo, path string) (sensorResp SensorDiscoveryInfo, err error) {
	sensorResp, err = tm.OneM2M_create(sensor, path, sensor.SensorIdentifier)
	if err != nil {
		return sensorResp, err
	}

	return sensorResp, nil
}

func oneM2M_createAE_CNT_CIN(tm *SssMgr) (sensors map[string]SensorDiscoveryInfo, received_sensors map[string]SensorDiscoveryInfo, err error) {
	sensors = make(map[string]SensorDiscoveryInfo, 0)
	received_sensors = make(map[string]SensorDiscoveryInfo, 0)

	var sensor_ae = SensorDiscoveryInfo{
		SensorIdentifier: "CMyAE",
		SensorType:       "AE",
		SensorPosition:   nil,
		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
	}
	sensors[sensor_ae.SensorType] = sensor_ae
	new_sensor_ae, err := oneM2M_create(tm, sensor_ae, "")
	if err != nil {
		return nil, nil, err
	}
	received_sensors[new_sensor_ae.SensorType] = new_sensor_ae

	var sensor_cnt = SensorDiscoveryInfo{
		SensorIdentifier: "CMyCNT",
		SensorType:       "CNT",
		SensorPosition:   nil,
		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
	}
	sensorPath := new_sensor_ae.SensorIdentifier
	sensors[sensor_cnt.SensorType] = sensor_cnt
	new_sensor_cnt, err := oneM2M_create(tm, sensor_cnt, sensorPath)
	if err != nil {
		return nil, nil, err
	}
	received_sensors[new_sensor_cnt.SensorType] = new_sensor_cnt

	var sensor_cin = SensorDiscoveryInfo{
		SensorIdentifier: "CMyCNI",
		SensorType:       "CIN",
		SensorPosition:   nil,
		IotPlatformId:    "0a692154-0f93-11f0-b554-df2f6a756f6a",
	}
	sensor_cin.SensorCharacteristicList = make([]SensorCharacteristic, 1)
	sensor_cin.SensorCharacteristicList[0] = SensorCharacteristic{CharacteristicName: "con", CharacteristicValue: "OFF"}
	sensorPath = new_sensor_cnt.SensorIdentifier
	sensors[sensor_cin.SensorType] = sensor_cin
	new_sensor_cin, err := oneM2M_create(tm, sensor_cin, sensorPath)
	if err != nil {
		return nil, nil, err
	}
	received_sensors[new_sensor_cin.SensorType] = new_sensor_cin

	return sensors, received_sensors, nil
}

func validate_sensor_discovery_info(expected_sensor SensorDiscoveryInfo, received_sensor SensorDiscoveryInfo) bool {
	fmt.Println(">>> validate_sensor_discovery_info: expected_sensor: ", expected_sensor)
	fmt.Println(">>> validate_sensor_discovery_info: received_sensor: ", received_sensor)

	if expected_sensor.SensorIdentifier == received_sensor.SensorIdentifier {
		fmt.Println("received_sensor.SensorIdentifier == SensorIdentifier")
		return false
	}
	if len(expected_sensor.SensorPropertyList) != len(received_sensor.SensorPropertyList) {
		fmt.Println("len(received_sensor.SensorPropertyList) != len(SensorPropertyList)")
		return false
	} else if !reflect.DeepEqual(expected_sensor.SensorPropertyList, received_sensor.SensorPropertyList) {
		fmt.Println("received_sensor.SensorPropertyList != SensorPropertyList")
	}
	if len(expected_sensor.SensorCharacteristicList) != len(received_sensor.SensorCharacteristicList) {
		fmt.Println("len(received_sensor.SensorCharacteristicList) != len(SensorCharacteristicList)")
		return false
	} else if !reflect.DeepEqual(expected_sensor.SensorCharacteristicList, received_sensor.SensorCharacteristicList) {
		fmt.Println("received_sensor.SensorCharacteristicList != SensorCharacteristicList")
	}
	// TODO To be continued

	return true
}

func validate_sensor_ae(expected_sensor SensorDiscoveryInfo, received_sensor SensorDiscoveryInfo) bool {
	fmt.Println(">>> validate_sensor_ae: expected_sensor: ", expected_sensor)
	fmt.Println(">>> validate_sensor_ae: received_sensor: ", received_sensor)

	if received_sensor.SensorIdentifier == "" {
		fmt.Println("validate_sensor_ae.SensorIdentifier shall be set")
		return false
	}
	if received_sensor.SensorType == "" {
		fmt.Println("validate_sensor_ae.SensorType shall be set")
		return false
	}
	if received_sensor.IotPlatformId != expected_sensor.IotPlatformId {
		fmt.Println("validate_sensor_ae.IotPlatformId != IotPlatformId")
		return false
	}
	if len(received_sensor.SensorCharacteristicList) == 0 {
		fmt.Println("validate_sensor_ae.SensorCharacteristicList shall not be empty")
		return false
	}

	for _, c := range received_sensor.SensorCharacteristicList { // FIXME FSCOM To be refined
		found := true
		switch c.CharacteristicName {
		case "api":
		case "srv":
		case "pi":
		case "ct":
		case "et":
		case "lt":
		case "aei":
		case "rn":
		case "rr":
		default:
			found = true
		} // End of 'switch' statement
		if !found {
			fmt.Println("Incomplete SensorCharacteristicList")
			return false
		}
	}

	return true
}

func validate_sensor_cnt(expected_sensor SensorDiscoveryInfo, received_sensor SensorDiscoveryInfo) bool {
	fmt.Println(">>> validate_sensor_cnt: expected_sensor: ", expected_sensor)
	fmt.Println(">>> validate_sensor_cnt: received_sensor: ", received_sensor)

	if received_sensor.SensorIdentifier == "" {
		fmt.Println("validate_sensor_cnt.SensorIdentifier shall be set")
		return false
	}
	if received_sensor.SensorType != received_sensor.SensorType {
		fmt.Println("validate_sensor_cnt.SensorType != SensorType")
		return false
	}
	if received_sensor.IotPlatformId != expected_sensor.IotPlatformId {
		fmt.Println("validate_sensor_cnt.IotPlatformId != IotPlatformId")
		return false
	}
	if len(received_sensor.SensorCharacteristicList) == 0 {
		fmt.Println("validate_sensor_cnt.SensorCharacteristicList shall not be empty")
		return false
	}

	for _, c := range received_sensor.SensorCharacteristicList { // FIXME FSCOM To be refined
		found := true
		switch c.CharacteristicName {
		case "cni":
		case "pi":
		case "ct":
		case "et":
		case "lt":
		case "st":
		case "cbs":
		case "mbs":
		case "mni":
		case "rn":
		default:
			found = true
		} // End of 'switch' statement
		if !found {
			fmt.Println("Incomplete SensorCharacteristicList")
			return false
		}
	}

	return true
}

func validate_sensor_cin(expected_sensor SensorDiscoveryInfo, received_sensor SensorDiscoveryInfo) bool {
	fmt.Println(">>> validate_sensor_cin: expected_sensor: ", expected_sensor)
	fmt.Println(">>> validate_sensor_cin: received_sensor: ", received_sensor)

	if received_sensor.SensorIdentifier == "" {
		fmt.Println("validate_sensor_ae.SensorIdentifier shall be set")
		return false
	}
	if received_sensor.SensorType != received_sensor.SensorType {
		fmt.Println("validate_sensor_cin.SensorType != SensorType")
		return false
	}
	if received_sensor.IotPlatformId != expected_sensor.IotPlatformId {
		fmt.Println("validate_sensor_cin.IotPlatformId != IotPlatformId")
		return false
	}
	if len(received_sensor.SensorCharacteristicList) == 0 {
		fmt.Println("validate_sensor_cin.SensorCharacteristicList shall not be empty")
		return false
	}

	for _, c := range received_sensor.SensorCharacteristicList { // FIXME FSCOM To be refined
		found := true
		switch c.CharacteristicName {
		case "pi":
		case "lt":
		case "cs":
		case "et":
		case "cnf":
		case "con":
		case "ct":
		case "rn":
		default:
			found = true
		} // End of 'switch' statement
		if !found {
			fmt.Println("Incomplete SensorCharacteristicList")
			return false
		}
	}

	return true
}

func validate_sensor_flex(expected_sensor SensorDiscoveryInfo, received_sensor SensorDiscoveryInfo) bool {
	fmt.Println(">>> validate_sensor_flex: expected_sensor: ", expected_sensor)
	fmt.Println(">>> validate_sensor_flex: received_sensor: ", received_sensor)

	if received_sensor.SensorIdentifier == "" {
		fmt.Println("validate_sensor_flex.SensorIdentifier shall be set")
		return false
	}
	if received_sensor.SensorType != received_sensor.SensorType {
		fmt.Println("validate_sensor_flex.SensorType != SensorType")
		return false
	}
	if received_sensor.IotPlatformId != expected_sensor.IotPlatformId {
		fmt.Println("validate_sensor_flex.IotPlatformId != IotPlatformId")
		return false
	}
	if expected_sensor.Flex == nil || received_sensor.Flex == nil {
		fmt.Println("validate_sensor_flex.Flex != Flex")
		return false
	} else if len(expected_sensor.Flex) != len(received_sensor.Flex) {
		fmt.Println("len(validate_sensor_flex.Flex) != len(Flex)")
		return false
	} else if expected_sensor.Flex["type"] != received_sensor.Flex["type"] {
		fmt.Println("validate_sensor_flex.Flex[type] != Flex[type]")
		return false
	} else if expected_sensor.Flex["cnd"] != received_sensor.Flex["cnd"] {
		fmt.Println("validate_sensor_flex.Flex[cnd] != Flex[cnd]")
		return false
	}

	return true
}

func discoveryNotify(map[string]interface{}) {
	log.Debug(">>> discoveryNotify")

}

func statusNotify(map[string]interface{}) {
	log.Debug(">>> statusNotify")

}

func dataNotify(map[string]interface{}) {
	log.Debug(">>> dataNotify")

}

func TestSecondIotPlatformKeepsFirstRegistration(t *testing.T) {
	log.MeepTextLogInit(t.Name())

	first, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "127.0.0.1", 18080, "platform-1", "cse-1", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSssMgr(tmName, tmNamespace, "REST_HTTP", "127.0.0.1", 18081, "platform-2", "cse-2", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.DeleteSssMgr()
	defer second.DeleteSssMgr()

	platform1, ok1 := platformById("platform-1")
	platform2, ok2 := platformById("platform-2")
	if !ok1 || !ok2 {
		t.Fatalf("both platforms should stay registered, got %v %v", ok1, ok2)
	}
	if platform1.Name != "cse-1" || platform1.Port != 18080 {
		t.Fatalf("first platform was overwritten: %+v", platform1)
	}
	if platform2.Name != "cse-2" || platform2.Port != 18081 {
		t.Fatalf("second platform was not stored: %+v", platform2)
	}
}
