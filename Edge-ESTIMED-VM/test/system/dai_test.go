/*
 * Copyright (c) 2024  ETSI STF 625
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
	"encoding/json"
	"fmt"
	//"strconv"
	"testing"

	"context"
	"time"

	daiClient "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-dai-client"
	meepdaimgr "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-dai-mgr"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

var daiAppClient *daiClient.APIClient
var daiServerUrl string

const (
	associateDevAppId1 = "associateDevAppId1"
	callbackReference1 = "callbackReference1"
	appName1           = "onboarded-demo4"
	appProvider1       = "ETSI"
	appDVersion1       = "v0.1.0"
	appDescription1    = "Basic HTTP Ping Pong"
	callbackReference2 = "callbackReference2"
)

var ( // Need to take address
	contextId1           string         = "onboarded-demo4"
	appDId1              string         = "onboarded-demo4"
	appSoftVersion1      string         = "v0.1.0"
	appPackageSource1    string         = "appPackageSource1"
	referenceURI1_1      meepdaimgr.Uri = "referenceURI1-1"
	area1                               = daiClient.Polygon{[][][]float32{{{7.420433, 43.729942}, {7.420659, 43.73036}, {7.420621, 43.731045}, {7.420922, 43.73129}}, {{7.420434, 43.729943}, {7.420659, 43.73036}, {7.420621, 43.731045}, {7.420922, 43.73129}}}}
	civicAddressElement1                = []daiClient.LocationConstraintsCivicAddressElement{{1, "Value1"}, {10, "Value10"}}
	countryCode1         string         = "33"
	memory1              int32          = 1024
	storage1             int32          = 1024
	latency1             int32          = 1024
	bandwidth1           int32          = 1024
	serviceCont1         int32          = 0
)

func init() {

	err := startSystemTest()
	if err != nil {
		log.Error("Cannot start system test: ", err)
	}
	//create client
	daiAppClientCfg := daiClient.NewConfiguration()
	if hostUrlStr == "" {
		hostUrlStr = "http://localhost"
	}

	daiAppClientCfg.BasePath = hostUrlStr + "/" + sandboxName + "/dev_app/v1"

	daiAppClient = daiClient.NewAPIClient(daiAppClientCfg)
	if daiAppClient == nil {
		log.Error("Failed to create DAI App REST API client: ", daiAppClientCfg.BasePath)
	}
	//NOTE: if localhost is set as the hostUrl, might not be reachable from the service, export MEEP_HOST_TEST_URL ="http://[yourhost]"
	daiServerUrl = hostUrlStr + ":" + httpListenerPort
}

func initialiseDaiTest() {
	log.Info("activating Scenario")
	err := activateScenario("dai-system-test")
	if err != nil {
		log.Fatal("Scenario cannot be activated: ", err)
	}
	time.Sleep(1000 * time.Millisecond)
	if isAutomationReady(true, 10, 0) {
		geAutomationUpdate(true, false, true, true)
	}
}

func clearUpDaiTest() {
	log.Info("terminating Scenario")
	terminateScenario()
	time.Sleep(1000 * time.Millisecond)
}

//no really a test, but loading the scenarios needed that will be used in the following tests
//deletion of those scenarios at the end
func Test_DAI_load_scenarios(t *testing.T) {

	// no override if the name is already in the DB.. security not to override something important
	err := createScenario("dai-system-test", "dai-system-test.yaml")
	if err != nil {
		t.Fatal("Cannot create scenario, keeping the one already there and continuing testing with it :", err)
	}
}

//not a real test, just the last test that stops the system test environment
func Test_DAI_stopSystemTest(t *testing.T) {
	err := deleteScenario("dai-system-test")
	if err != nil {
		log.Error("cannot delete scenario :", err)
	}
}

func Test_DAI_MeAppListGET_no_params(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	initialiseVisTest()
	defer clearUpVisTest()

	// Fill LocationConstraints table
	appLocationConstraints := make([]daiClient.LocationConstraints, 1)
	appLocationConstraints[0] = daiClient.LocationConstraints{nil, nil, countryCode1}
	// Fill ApplicationListAppInfo
	var appInfo daiClient.ApplicationListAppInfo
	appInfo.AppDId = appDId1
	appInfo.AppDVersion = appDVersion1
	appInfo.AppDescription = appDescription1
	appInfo.AppLocation = appLocationConstraints
	appInfo.AppName = appName1
	appInfo.AppProvider = appProvider1
	appInfo.AppSoftVersion = appSoftVersion1
	// Fill ApplicationListAppList
	var applicationListAppList daiClient.ApplicationListAppList
	applicationListAppList.AppInfo = &appInfo
	applicationListAppList.VendorSpecificExt = nil
	appInfo.AppCharcs = new(daiClient.ApplicationListAppInfoAppCharcs)
	appInfo.AppCharcs.Memory = memory1
	appInfo.AppCharcs.Storage = storage1
	appInfo.AppCharcs.Latency = latency1
	appInfo.AppCharcs.Bandwidth = bandwidth1
	appInfo.AppCharcs.ServiceCont = serviceCont1
	// ApplicationList
	var expectedAppInfoList daiClient.ApplicationList
	expectedAppInfoList.AppList = make([]daiClient.ApplicationListAppList, 1)
	expectedAppInfoList.AppList[0] = applicationListAppList
	fmt.Println("expectedAppInfoList: ", expectedAppInfoList)

	// Request to test with time set to 08:47:00 for congestion
	err := daiMeAppListGETAll()
	if err != nil {
		t.Fatal("MeAppListGET failed: ", err)
	}

	if len(httpReqBody) > 1 {
		var body daiClient.ApplicationList
		err = json.Unmarshal([]byte(httpReqBody[0]), &body)
		if err != nil {
			t.Fatalf("cannot unmarshall response")
		}
		errStr := validateApplicationList(&body, expectedAppInfoList)
		if errStr != "" {
			printHttpReqBody()
			t.Fatalf(errStr)
		}
	} else {
		printHttpReqBody()
		t.Fatalf("Number of expected notifications not received")
	}
}

func daiMeAppListGETAll() error {
	_, _, err := daiAppClient.DevAppApi.MeAppListGET(context.TODO(), nil)
	if err != nil {
		log.Error("Failed to send subscription: ", err)
		return err
	}

	return nil
}

func validateApplicationList(appInfoListEntry *daiClient.ApplicationList, appInfoList daiClient.ApplicationList) string {

	if appInfoListEntry == nil {
		return "appInfoListEntry == nil"
	}

	if len(appInfoListEntry.AppList) != len(appInfoList.AppList) {
		return "len(appInfoListEntry.AppList) != len(appInfoList.AppList)"
	}

	return ""
}
