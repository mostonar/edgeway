/*
 * Copyright (c) 2022 ETSI STF 625
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

package systemTest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	visClient "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-vis-client"
)

var visAppClient *visClient.APIClient
var visServerUrl string

func init() {

	err := startSystemTest()
	if err != nil {
		log.Error("Cannot start system test: ", err)
	}
	//create client
	visAppClientCfg := visClient.NewConfiguration()
	if hostUrlStr == "" {
		hostUrlStr = "http://localhost"
	}

	visAppClientCfg.BasePath = hostUrlStr + "/" + sandboxName + "/vis/v2"

	visAppClient = visClient.NewAPIClient(visAppClientCfg)
	if visAppClient == nil {
		log.Error("Failed to create VIS App REST API client: ", visAppClientCfg.BasePath)
	}
	//NOTE: if localhost is set as the hostUrl, might not be reachable from the service, export MEEP_HOST_TEST_URL ="http://[yourhost]"
	visServerUrl = hostUrlStr + ":" + httpListenerPort
}

func initialiseVisTest() {
	log.Info("activating Scenario")
	err := activateScenario("vis-system-test")
	if err != nil {
		log.Fatal("Scenario cannot be activated: ", err)
	}
	time.Sleep(1000 * time.Millisecond)
	if isAutomationReady(true, 10, 0) {
		geAutomationUpdate(true, false, true, true)
	}
}

func clearUpVisTest() {
	log.Info("terminating Scenario")
	terminateScenario()
	time.Sleep(1000 * time.Millisecond)
}

//no really a test, but loading the scenarios needed that will be used in the following tests
//deletion of those scenarios at the end
func Test_VIS_load_scenarios(t *testing.T) {

	// no override if the name is already in the DB.. security not to override something important
	err := createScenario("vis-system-test", "vis-system-test.yaml")
	if err != nil {
		t.Fatal("Cannot create scenario, keeping the one already there and continuing testing with it :", err)
	}
}

//not a real test, just the last test that stops the system test environment
func Test_VIS_stopSystemTest(t *testing.T) {
	err := deleteScenario("vis-system-test")
	if err != nil {
		log.Error("cannot delete scenario :", err)
	}
}

func Test_VIS_with_prediction_model(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	initialiseVisTest()
	defer clearUpVisTest()

	// Initialize the data structure for the POST request
	// See https://docbox.etsi.org/ISG/MEC/DECODE/05-CONTRIBUTIONS/2022//MECDECODE(22)000044_Demo_of_MEC-030_V2X_Information_Service_API_in_MEC_Sandbox.pptx Slide #9
	pointA := visClient.LocationInfoGeoArea{Longitude: 7.414853, Latitude: 43.729416}
	locationInfoA := visClient.LocationInfo{GeoArea: &pointA}
	tsA := visClient.TimeStamp{NanoSeconds: 0, Seconds: 1653295620} // Using Prediction model
	pointB := visClient.LocationInfoGeoArea{Longitude: 7.418417, Latitude: 43.732456}
	locationInfoB := visClient.LocationInfo{GeoArea: &pointB}
	tsB := visClient.TimeStamp{NanoSeconds: 0, Seconds: 1653295620} // Using Prediction model
	// Fill PredictedQosRoutesRouteInfo with LocationInfo list
	routeInfo := make([]visClient.PredictedQosRoutesRouteInfo, 2)
	routeInfo[0] = visClient.PredictedQosRoutesRouteInfo{
		Location: &locationInfoA,
		Rsrq:     0,
		Rsrp:     0,
		Time:     &tsA,
	}
	routeInfo[1] = visClient.PredictedQosRoutesRouteInfo{
		Location: &locationInfoB,
		Rsrq:     0,
		Rsrp:     0,
		Time:     &tsB,
	}
	// PredictedQosRoutes with PredictedQosRoutesRouteInfo list
	predictedQosRoutes := visClient.PredictedQosRoutes{RouteInfo: routeInfo}
	// Fill PredictedQos with PredictedQosRoutes list
	routes := make([]visClient.PredictedQosRoutes, 1)
	routes[0] = predictedQosRoutes
	predictedQos := visClient.PredictedQos{
		LocationGranularity: "1",
		Routes:              routes,
	}
	fmt.Println("predictedQos: ", predictedQos)
	testPredictedQos := visClient.PredictedQos{
		LocationGranularity: "1",
		Routes:              routes,
	}
	testPredictedQos.Routes[0].RouteInfo[0].Rsrq = 20
	testPredictedQos.Routes[0].RouteInfo[0].Rsrp = 60
	testPredictedQos.Routes[0].RouteInfo[1].Rsrq = 13
	testPredictedQos.Routes[0].RouteInfo[1].Rsrp = 55
	fmt.Println("testPredictedQos: ", predictedQos)

	// Moving to initial position: 4g-macro-cell-2/Residential
	testAddress := "ue1"
	geMoveAssetCoordinates(testAddress, 7.414853, 43.729416)
	time.Sleep(2000 * time.Millisecond)

	// Request to test with time set to 08:47:00 for congestion
	err := visPredictedQoSPOST(predictedQos)
	if err != nil {
		t.Fatal("PredictedQoS Post failed: ", err)
	}

	if len(httpReqBody) > 1 {
		var body visClient.PredictedQos
		err = json.Unmarshal([]byte(httpReqBody[0]), &body)
		if err != nil {
			t.Fatalf("cannot unmarshall response")
		}
		errStr := validatePredictedQos(&body, &testPredictedQos)
		if errStr != "" {
			printHttpReqBody()
			t.Fatalf(errStr)
		}

	} else {
		printHttpReqBody()
		t.Fatalf("Number of expected notifications not received")
	}

}

func Test_VIS_without_prediction_model(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	initialiseVisTest()
	defer clearUpVisTest()

	// Initialize the data structure for the POST request
	// See https://docbox.etsi.org/ISG/MEC/DECODE/05-CONTRIBUTIONS/2022//MECDECODE(22)000044_Demo_of_MEC-030_V2X_Information_Service_API_in_MEC_Sandbox.pptx Slide #9
	pointA := visClient.LocationInfoGeoArea{Longitude: 7.414853, Latitude: 43.729416}
	locationInfoA := visClient.LocationInfo{GeoArea: &pointA}
	pointB := visClient.LocationInfoGeoArea{Longitude: 7.418417, Latitude: 43.732456}
	locationInfoB := visClient.LocationInfo{GeoArea: &pointB}
	// Fill PredictedQosRoutesRouteInfo with LocationInfo list
	routeInfo := make([]visClient.PredictedQosRoutesRouteInfo, 2)
	routeInfo[0] = visClient.PredictedQosRoutesRouteInfo{
		Location: &locationInfoA,
		Rsrq:     0,
		Rsrp:     0,
		Time:     nil,
	}
	routeInfo[1] = visClient.PredictedQosRoutesRouteInfo{
		Location: &locationInfoB,
		Rsrq:     0,
		Rsrp:     0,
		Time:     nil,
	}
	// PredictedQosRoutes with PredictedQosRoutesRouteInfo list
	predictedQosRoutes := visClient.PredictedQosRoutes{RouteInfo: routeInfo}
	// Fill PredictedQos with PredictedQosRoutes list
	routes := make([]visClient.PredictedQosRoutes, 1)
	routes[0] = predictedQosRoutes
	predictedQos := visClient.PredictedQos{
		LocationGranularity: "1",
		Routes:              routes,
	}
	fmt.Println("predictedQos: ", predictedQos)
	testPredictedQos := visClient.PredictedQos{
		LocationGranularity: "1",
		Routes:              routes,
	}
	testPredictedQos.Routes[0].RouteInfo[0].Rsrq = 21
	testPredictedQos.Routes[0].RouteInfo[0].Rsrp = 63
	testPredictedQos.Routes[0].RouteInfo[1].Rsrq = 13
	testPredictedQos.Routes[0].RouteInfo[1].Rsrp = 55
	fmt.Println("testPredictedQos: ", predictedQos)

	// Moving to initial position: 4g-macro-cell-2/Residential
	testAddress := "ue1"
	geMoveAssetCoordinates(testAddress, 7.414853, 43.729416)
	time.Sleep(2000 * time.Millisecond)

	// Request to test with time set to 08:47:00 for congestion
	err := visPredictedQoSPOST(predictedQos)
	if err != nil {
		t.Fatal("PredictedQoS Post failed: ", err)
	}

	if len(httpReqBody) > 1 {
		var body visClient.PredictedQos
		err = json.Unmarshal([]byte(httpReqBody[0]), &body)
		if err != nil {
			t.Fatalf("cannot unmarshall response")
		}
		errStr := validatePredictedQos(&body, &testPredictedQos)
		if errStr != "" {
			printHttpReqBody()
			t.Fatalf(errStr)
		}

	} else {
		printHttpReqBody()
		t.Fatalf("Number of expected notifications not received")
	}

}

func visPredictedQoSPOST(testPredictedQos visClient.PredictedQos) error {
	//PredictedQosPOST(ctx context.Context, body PredictedQos) (PredictedQos, *http.Response, error)
	_, _, err := visAppClient.V2xiApi.PredictedQosPOST(context.TODO(), testPredictedQos)
	if err != nil {
		log.Error("Failed to send subscription: ", err)
		return err
	}

	return nil
}

func validatePredictedQos(response *visClient.PredictedQos, expected_response *visClient.PredictedQos) string {

	if response.LocationGranularity != expected_response.LocationGranularity {
		return ("PredictedQos.LocationGranularity not as expected: " + response.LocationGranularity + " instead of " + expected_response.LocationGranularity)
	}
	if len(response.Routes) != len(expected_response.Routes) {
		return ("response.Routes not as expected")
	}
	if len(response.Routes[0].RouteInfo) != len(expected_response.Routes[0].RouteInfo) {
		return ("response.Routes[0].RouteInfo not as expected")
	}

	var s string = ""
	for _, item := range response.Routes[0].RouteInfo {
		found := false
		for _, item1 := range expected_response.Routes[0].RouteInfo {
			if item.Location != nil && item1.Location != nil {
				if item.Location.GeoArea != nil && item1.Location.GeoArea != nil {
					if item.Location.GeoArea.Latitude == item1.Location.GeoArea.Latitude && item.Location.GeoArea.Longitude == item1.Location.GeoArea.Longitude {
						if item.Rsrp == item1.Rsrp && item.Rsrq == item1.Rsrq {
							// Found it
							found = true
							break
						} else {
							s = ("response.Rsrx != Rsrx")
						}
					} else {
						s = ("response.Latitude/Longitude != Latitude/Longitude")
					}
				} else {
					s = ("response.GeoArea != GeoArea")
				}
			} else {
				s = ("response.Location != Location")
			}
		} // End of 'for' statement
		if !found {
			break
		}
	}

	return s
}
