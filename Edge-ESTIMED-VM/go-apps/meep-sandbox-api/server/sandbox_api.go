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
	//"bytes"
	"context"
	"crypto/tls"
	"io/ioutil"
	"os"
	"strings"

	//"crypto/rand"
	///"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"net/http"
	"net/url"

	"strconv"
	//"sync"
	"time"

	gis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-engine-client"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	met "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	platformCtrlClient "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-platform-ctrl-client"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	sandboxCtrlClient "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client"
	sm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sessions"
	users "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-users"

	auth "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-auth-svc-client"

	//  "github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-auth-svc"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	//  "github.com/InterDigitalInc/AdvantEDGE/go-apps/meepctl/utils"
)

// const moduleName = "meep-sandbox-api"
const redisTable = 0

// const serviceName = "Sandbox API"
const moduleName = "meep-sandbox-api"
const moduleNamespace = "default"

var baseKey string

const authBasepath = "http://meep-auth-svc/auth/v1"
const pfmCtrlBasepath = "http://meep-platform-ctrl/platform-ctrl/v1"

type SandboxApiConnectors struct {
	sessionMgr           *sm.SessionMgr
	userStore            *users.Connector
	metricStore          *met.MetricStore
	rc                   *redis.Connector
	mqGlobal             *mq.MsgQueue
	authClient           *auth.APIClient
	pfmCtrlClient        *platformCtrlClient.APIClient
	sandboxCtrlAppClient map[string]*sandboxCtrlClient.APIClient
	sessionStore         map[string]*sm.Session
	gisClient            map[string]*gis.APIClient
	uri                  *url.URL
}

type DeviceOauth struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
}

type SandboxName struct {
	SandboxName string `json:"sandbox_name"`
}

var sandboxApiConnectors *SandboxApiConnectors

/*
*************************************************************************************************************************
// FIXME All the function below shall be done by AuthSvc login procedure. Currently, it is bypassed for the purpose of this PoC
*************************************************************************************************************************
*/
var postgisUser = "postgres"
var postgisPwd = "pwd"

var postgisHost string = ""
var postgisPort string = ""

var influxAddr string = "http://meep-influxdb.default.svc.cluster.local:8086"

// Metrics
var (
	metricSessionLogin = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auth_svc_session_login_total",
		Help: "The total number of session login attempts",
	}, []string{"type"})
	metricSessionLogout = promauto.NewCounter(prometheus.CounterOpts{
		Name: "auth_svc_session_logout_total",
		Help: "The total number of session logout attempts",
	})
	metricSessionSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "auth_svc_session_success_total",
		Help: "The total number of successful sessions",
	})
	metricSessionFail = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "auth_svc_session_fail_total",
		Help: "The total number of failed session login attempts",
	}, []string{"type"})
	metricSessionTimeout = promauto.NewCounter(prometheus.CounterOpts{
		Name: "auth_svc_session_timeout_total",
		Help: "The total number of timed out sessions",
	})
	metricSessionActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "auth_svc_session_active",
		Help: "The number of active sessions",
	})
	metricSessionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "auth_svc_session_duration",
		Help:    "A histogram of session durations",
		Buckets: prometheus.LinearBuckets(20, 20, 6),
	})
)

// Declare as variables to enable overwrite in test
var redisAddr = "meep-redis-master:6379"

var Ues map[string]map[string][]sandboxCtrlClient.PhysicalLocation
var Ues_count map[string]map[string]int32

// Init - RNI Service initialization
func Init() (err error) {
	log.Debug(">>> Init")

	// Create new Platform Controller
	sandboxApiConnectors = new(SandboxApiConnectors)

	// Create message queue
	sandboxApiConnectors.mqGlobal, err = mq.NewMsgQueue(mq.GetGlobalName(), moduleName, moduleNamespace, redisAddr)
	if err != nil {
		log.Error("Failed to create Message Queue with error: ", err)
		return err
	}
	log.Info("Message Queue created")

	// Connect to Redis DB
	sandboxApiConnectors.rc, err = redis.NewConnector(redisAddr, redisTable)
	if err != nil {
		log.Error("Failed connection to Redis DB. Error: ", err)
		return err
	}
	log.Info("Connected to Redis DB")

	// Create Auth-Svc REST API client
	authClientCfg := auth.NewConfiguration()
	authClientCfg.BasePath = authBasepath
	sandboxApiConnectors.authClient = auth.NewAPIClient(authClientCfg)
	if sandboxApiConnectors.authClient == nil {
		err := errors.New("Failed to create Auth Svc REST API client")
		return err
	}
	log.Info("Auth Svc REST API client created")

	// Create Platform Controller REST API client
	pfmCtrlClientCfg := platformCtrlClient.NewConfiguration()
	pfmCtrlClientCfg.BasePath = pfmCtrlBasepath
	sandboxApiConnectors.pfmCtrlClient = platformCtrlClient.NewAPIClient(pfmCtrlClientCfg)
	if sandboxApiConnectors.pfmCtrlClient == nil {
		err := errors.New("Failed to create Platform Ctrl REST API client")
		return err
	}
	log.Info("Platform Ctrl REST API client created")

	// Connect to Session Manager
	sandboxApiConnectors.sessionMgr, err = sm.NewSessionMgr(moduleName, "", redisAddr, redisAddr)
	if err != nil {
		log.Error("Failed connection to Session Manager: ", err.Error())
		return err
	}
	log.Info("Connected to Session Manager")

	// Connect to User Store
	sandboxApiConnectors.userStore, err = users.NewConnector(moduleName, postgisUser, postgisPwd, postgisHost, postgisPort)
	if err != nil {
		log.Error("Failed connection to User Store: ", err.Error())
		return err
	}
	log.Info("Connected to User Store")

	// Connect to Metric Store
	sandboxApiConnectors.metricStore, err = met.NewMetricStore("session-metrics", "global", influxAddr, met.MetricsDbDisabled)
	if err != nil {
		log.Error("Failed connection to Metric Store: ", err)
		return err
	}

	// hostUrl is the url of the node serving the resourceURL
	// Retrieve public url address where service is reachable, if not present, use Host URL environment variable
	sandboxApiConnectors.uri, err = url.Parse(strings.TrimSpace(os.Getenv("MEEP_PUBLIC_URL")))
	if err != nil || sandboxApiConnectors.uri == nil || sandboxApiConnectors.uri.String() == "" {
		sandboxApiConnectors.uri, err = url.Parse(strings.TrimSpace(os.Getenv("MEEP_HOST_URL")))
		if err != nil {
			sandboxApiConnectors.uri = new(url.URL)
		}
	}
	log.Info("sandboxApiConnectors.uri: ", sandboxApiConnectors.uri)

	sandboxApiConnectors.sessionStore = make(map[string]*sm.Session)
	sandboxApiConnectors.sandboxCtrlAppClient = make(map[string]*sandboxCtrlClient.APIClient)
	sandboxApiConnectors.gisClient = make(map[string]*gis.APIClient)
	Ues = make(map[string]map[string][]sandboxCtrlClient.PhysicalLocation)
	Ues_count = make(map[string]map[string]int32)

	log.Info("SANDBOX_API successfully initialized")
	return nil
}

// reInit - finds the value already in the DB to repopulate local stored info
// func reInit() {
// 	//keyName := baseKey + "subscriptions:" + "*"
// }

// Run - Start SANDBOX_API
func Run() (err error) {
	log.Debug(">>> Run")

	/**************************************************************************************************************************
	// FIXME All the function below shall be done by AuthSvc login procedure. Currently, it is bypassed for the purpose of this PoC
	**************************************************************************************************************************/
	// Start Session Watchdog
	err = sandboxApiConnectors.sessionMgr.StartSessionWatchdog(sessionTimeoutCb)
	if err != nil {
		log.Error("Failed start Session Watchdog: ", err.Error())
		return err
	}
	/**************************************************************************************************************************
	 **************************************************************************************************************************/

	return nil
}

// Stop - Stop SANDBOX_API
func Stop() (err error) {
	log.Debug(">>> Stop")

	return nil
}

/*
 * errHandlerProblemDetails sends an error message
 * @param {struct} HTTP write reference
 * @param {string} error contains the error message
 * @param {int} code contains the error code
 */
func errHandlerProblemDetails(w http.ResponseWriter, error string, code int) {
	var pb ProblemDetails
	pb.Detail = error
	pb.Status = int32(code)

	jsonResponse := convertProblemDetailstoJson(&pb)

	w.WriteHeader(code)
	fmt.Fprint(w, jsonResponse)
}

func login(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> Login: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	found := false
	q := u.Query()
	for param := range q {
		if param == "provider" {
			found = true
			break
		}
	} // End of 'for' statement
	if !found {
		err := errors.New("Wrong parameters: provider")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Get the query parameter provider
	provider := q.Get("provider")
	if provider != "github" && provider != "Jupyter2024" { // FXIME FSCOM Debug urpose: bypass authent/Author
		err := errors.New("Wrong provider Value, only github is permitted")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("Login: provider: ", provider)

	/**************************************************************************************************************************
	// FIXME All the function below shall be done by AuthSvc login procedure. Currently, it is bypassed for the purpose of this PoC
	**************************************************************************************************************************/
	metricSessionLogin.WithLabelValues("Basic").Inc()

	userId := "toto"
	createSandbox := true

	var metric met.SessionMetric
	metric.Provider = provider
	metric.User = userId

	if provider == "Jupyter2024" { // FXIME FSCOM Debug purpose: bypass authent/Author
		log.Info("Start user session")
		sandboxName, isNew, _, err, _ := startSession(provider, userId, w, r, createSandbox)
		if err != nil {
			log.Error(err.Error())
			metricSessionFail.WithLabelValues("Session").Inc()
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		metric.Sandbox = sandboxName
		_ = sandboxApiConnectors.metricStore.SetSessionMetric(met.SesMetTypeLogin, metric)

		sandboxCtrlAppClientCfg := sandboxCtrlClient.NewConfiguration()
		sandboxCtrlAppClientCfg.BasePath = sandboxApiConnectors.uri.String() + "/" + sandboxName + "/sandbox-ctrl/v1"
		log.Info("sandboxCtrlAppClientCfg.BasePath: ", sandboxCtrlAppClientCfg.BasePath)
		tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		sandboxCtrlAppClientCfg.HTTPClient = &http.Client{Transport: tr}
		log.Info("sandboxCtrlAppClientCfg.HTTPClient: ", sandboxCtrlAppClientCfg.HTTPClient)
		sandboxApiConnectors.sandboxCtrlAppClient[sandboxName] = sandboxCtrlClient.NewAPIClient(sandboxCtrlAppClientCfg)
		if sandboxApiConnectors.sandboxCtrlAppClient[sandboxName] == nil {
			log.Error("Failed to create Sandbox Ctrl REST API client")
			metricSessionFail.WithLabelValues("Session").Inc()
			_ = processLogout(sandboxName, w, r)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Connect to GIS engine
		gisClientCfg := gis.NewConfiguration()
		gisClientCfg.BasePath = sandboxApiConnectors.uri.String() + "/" + sandboxName + "/gis/v1"
		log.Info("gisClientCfg.BasePath: ", gisClientCfg.BasePath)
		gisClientCfg.HTTPClient = &http.Client{Transport: tr}
		log.Info("gisClientCfg.HTTPClient: ", gisClientCfg.HTTPClient)
		sandboxApiConnectors.gisClient[sandboxName] = gis.NewAPIClient(gisClientCfg)
		if sandboxApiConnectors.gisClient[sandboxName] == nil {
			log.Error("Failed to create Sandbox GIS REST API client")
			_ = processLogout(sandboxName, w, r)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		metricSessionSuccess.Inc()
		if isNew {
			metricSessionActive.Inc()
		}

		deviceOauth := DeviceOauth{
			UserCode:        sandboxName,
			VerificationURI: "",
		}
		sandboxInfo := SandboxName{
			SandboxName: sandboxName,
		}
		jsonResponseToSave, _ := json.Marshal(sandboxInfo)

		timeInHours := 8
		timeInSeconds := timeInHours * 3600
		_ = sandboxApiConnectors.rc.JSONSetEntryWithExpiry(baseKey+":"+sandboxName, string(jsonResponseToSave), timeInSeconds)
		log.Info("Sandbox Info is stored into the Redis DB")

		// Format response
		jsonResponse, _ := json.Marshal(deviceOauth)
		log.Info("Response body:  ", string(jsonResponse))
		// Send response
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, string(jsonResponse))

		return
	}

	clientID := strings.TrimSpace(os.Getenv("MEEP_OAUTH_GITHUB_CLIENT_ID"))
	requestUrl := strings.TrimSpace(os.Getenv("MEEP_OAUTH_GITHUB_AUTH_URL"))
	pollingRequestUrl := strings.TrimSpace(os.Getenv("MEEP_OAUTH_GITHUB_POLL_URL"))
	log.Info("Client ID is ", clientID)
	log.Info("REQUEST URL is ", requestUrl)
	log.Info("POLLING REQUEST URL is ", pollingRequestUrl)

	// Prepare the request body
	body := strings.NewReader(fmt.Sprintf(`{"client_id": "%s"}`, clientID))

	// Create a new HTTP request
	req, err := http.NewRequest("POST", requestUrl, body)
	if err != nil {
		log.Info("Error creating request:", err)
		return
	}

	// Set request headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)

	if err != nil {
		log.Info("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Info("Error reading response body:", err)
		return
	}
	log.Info("Initial Response Body: ", string(respBody))

	// Parse the response body into url.Values
	params, err := url.ParseQuery(string(respBody))
	if err != nil {
		log.Info("Error parsing query parameters:", err)
		return
	}

	// Extract values from parsed query parameters
	deviceCode := params.Get("device_code")
	//  expiresIn := params.Get("expires_in")
	interval := params.Get("interval")
	userCode := params.Get("user_code")
	verificationURI := params.Get("verification_uri")

	// Print the variables
	log.Info("Device Code:", deviceCode)
	log.Info("User Code:", userCode)
	log.Info("Verification URI:", verificationURI)

	deviceOauth := DeviceOauth{
		UserCode:        userCode,
		VerificationURI: verificationURI,
	}
	// Format response
	jsonResponse, _ := json.Marshal(deviceOauth)
	log.Info("Response body:  ", string(jsonResponse))

	go func() {
		attempts := 0
		var accessToken string
		maxAttempts := 180

		for attempts < maxAttempts {
			// Prepare the request body
			pollingBody := strings.NewReader(fmt.Sprintf(`{"client_id": "%s", "device_code": "%s", "grant_type": "urn:ietf:params:oauth:grant-type:device_code"}`, clientID, deviceCode))

			// Create a new HTTP request
			pollingReq, err := http.NewRequest("POST", pollingRequestUrl, pollingBody)
			if err != nil {
				log.Info("Error creating request:", err)
				return
			}
			// Set request headers
			pollingReq.Header.Set("Content-Type", "application/json")

			// Send the request
			pollingResp, err := client.Do(pollingReq)
			if err != nil {
				log.Info("Error sending request:", err)
				return
			}
			defer pollingResp.Body.Close()

			// Read the response body
			pollingRespBody, err := ioutil.ReadAll(pollingResp.Body)
			if err != nil {
				log.Info("Error reading response body:", err)
				return
			}

			// Check if response contains access_token
			responseString := string(pollingRespBody)
			if strings.Contains(responseString, "access_token") {
				// Extract access token
				accessToken = strings.Split(responseString, "=")[1]
				log.Info("Access token:", accessToken)
				break
			} else {
				log.Info("Access token not found. Retrying...")
			}

			// Increment attempts
			attempts++

			// Wait for 5 seconds before making the next attempt
			intervalInt, _ := strconv.Atoi(interval)
			time.Sleep(time.Duration(intervalInt) * time.Second)
		}

		if accessToken == "" {
			log.Error("Failed to retrieve access token after maximum attempts.")
			return
		} else {
			log.Info("Successfully retrieved access token:", accessToken)
		}

		log.Info("Start user session")
		sandboxName, isNew, _, err, _ := startSession(provider, userId, w, r, createSandbox)
		if err != nil {
			log.Error(err.Error())
			metricSessionFail.WithLabelValues("Session").Inc()
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sandboxInfo := SandboxName{
			SandboxName: sandboxName,
		}
		jsonResponseToSave, _ := json.Marshal(sandboxInfo)

		metric.Sandbox = sandboxName
		_ = sandboxApiConnectors.metricStore.SetSessionMetric(met.SesMetTypeLogin, metric)

		sandboxCtrlAppClientCfg := sandboxCtrlClient.NewConfiguration()
		sandboxCtrlAppClientCfg.BasePath = sandboxApiConnectors.uri.String() + "/" + sandboxName + "/sandbox-ctrl/v1"
		log.Info("sandboxCtrlAppClientCfg.BasePath: ", sandboxCtrlAppClientCfg.BasePath)
		tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		sandboxCtrlAppClientCfg.HTTPClient = &http.Client{Transport: tr}
		log.Info("sandboxCtrlAppClientCfg.HTTPClient: ", sandboxCtrlAppClientCfg.HTTPClient)
		sandboxApiConnectors.sandboxCtrlAppClient[sandboxName] = sandboxCtrlClient.NewAPIClient(sandboxCtrlAppClientCfg)
		if sandboxApiConnectors.sandboxCtrlAppClient[sandboxName] == nil {
			log.Error("Failed to create Sandbox Ctrl REST API client")
			metricSessionFail.WithLabelValues("Session").Inc()
			_ = processLogout(sandboxName, w, r)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Connect to GIS engine
		gisClientCfg := gis.NewConfiguration()
		gisClientCfg.BasePath = sandboxApiConnectors.uri.String() + "/" + sandboxName + "/gis/v1"
		log.Info("gisClientCfg.BasePath: ", gisClientCfg.BasePath)
		gisClientCfg.HTTPClient = &http.Client{Transport: tr}
		log.Info("gisClientCfg.HTTPClient: ", gisClientCfg.HTTPClient)
		sandboxApiConnectors.gisClient[sandboxName] = gis.NewAPIClient(gisClientCfg)
		if sandboxApiConnectors.gisClient[sandboxName] == nil {
			log.Error("Failed to create Sandbox GIS REST API client")
			_ = processLogout(sandboxName, w, r)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		metricSessionSuccess.Inc()
		if isNew {
			metricSessionActive.Inc()
		}

		timeInSeconds := 30
		_ = sandboxApiConnectors.rc.JSONSetEntryWithExpiry(baseKey+":"+userCode, string(jsonResponseToSave), timeInSeconds)
		log.Info("Sandbox Info is stored into the Redis DB")

	}()
	// Send response
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, string(jsonResponse))
}

func apiConsoleLogsGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> GET HTTP Logs of the activated scenario: ", r)

	// Retrieve path parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	path := u.Path
	log.Info("Full path: ", path)

	// Split the path to get the path parameter
	pathParts := strings.Split(path, "/")
	sandboxName := pathParts[len(pathParts)-1]

	log.Info("Path parameter SANDBOX at the end is: ", sandboxName)

	targetSubstring := sandboxName
	found := false

	dbNames, _ := sandboxApiConnectors.metricStore.GetDbsInInfluxDb()

	var dbName string
	for _, db := range dbNames {
		if name, ok := db["name"]; ok {
			var strName string
			if s, ok := name.(string); ok {
				strName = s
			} else {
				strName = fmt.Sprintf("%v", name)
			}
			if strings.Contains(strName, targetSubstring) {
				dbName = strName
				found = true
				break
			}
		}
	}
	log.Info("DB NAME IS: ", dbName)

	if found {
		log.Info(fmt.Sprintf("DB Name %s is present in the DB List", dbName))
		metricsResult, _ := sandboxApiConnectors.metricStore.GetHttpMetricWithDbName(dbName, moduleName, "notification", "", 10)
		log.Info("METRICS in the InfluxDB ARE: ", metricsResult)

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, metricsResult)
		log.Info("METRICS in the InfluxDB ARE: ", metricsResult)
		return
	} else {
		msg := fmt.Sprintf("No active Scenario found against Sandbox: %s", sandboxName)
		log.Error(msg)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, msg)
		return
	}
}

func getNamespace(w http.ResponseWriter, r *http.Request) {

	log.Debug(">>> GET Namespace: ", r)

	var deviceOauth DeviceOauth

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	found := false
	q := u.Query()
	for param := range q {
		if param == "user_code" {
			found = true
			break
		}
	} // End of 'for' statement
	if !found {
		err := errors.New("Wrong parameters: provider")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	userCode := q.Get("user_code")
	log.Info("User Code is: ", userCode)

	jsonInfo, _ := sandboxApiConnectors.rc.JSONGetEntry(baseKey+":"+userCode, ".")
	if jsonInfo == "" {
		msg := "User is not Authenticated or the access token associated with user_code is expired " + userCode
		log.Error(msg)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, msg)
		return

	}
	log.Info("Sandbox Info obtained from the Redis DB", jsonInfo)

	err := json.Unmarshal([]byte(jsonInfo), &deviceOauth)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonInfo))
}

func logout(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> Logout: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	found := false
	q := u.Query()
	for param := range q {
		if param == "sandbox_name" {
			found = true
			break
		}
	} // End of 'for' statement
	if !found {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	sandbox_name := q.Get("sandbox_name")
	log.Info("Logout: sandbox_name: ", sandbox_name)

	//_ = sandboxApiConnectors.authClient.AuthApi.Logout(context.TODO())
	err := processLogout(sandbox_name, w, r)
	if err != nil {
		return
	}

	deleteUesIfAny(sandbox_name)

	w.WriteHeader(http.StatusNoContent)
}

func sandboxNetworkScenariosGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxNetworkScenariosGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve path parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve query parameters
	q := u.Query()
	log.Info("q: ", q)
	if q["sandbox_name"] == nil || len(q["sandbox_name"]) == 0 {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	sandbox_name := q["sandbox_name"][0]
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	scenarioList, resp, err := sandboxApiConnectors.pfmCtrlClient.ScenarioConfigurationApi.GetScenarioList(context.TODO())
	if err != nil {
		log.Error(err.Error())
		status := http.StatusBadRequest
		if resp != nil && resp.StatusCode >= 400 {
			status = resp.StatusCode
		}
		w.WriteHeader(status)
		return
	}
	log.Info("sandboxNetworkScenariosGET: resp: ", resp)
	log.Info("sandboxNetworkScenariosGET: scenarioList: ", scenarioList)

	l := make([]SandboxNetworkScenario, len(scenarioList.Scenarios))
	for i, scenario := range scenarioList.Scenarios {
		l[i].Id = scenario.Name
	} // End of 'for' statement

	// Marshalling
	jsonResponse, err := json.Marshal(l)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("jsonResponse: ", string(jsonResponse))

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func sandboxIndividualNetworkScenariosGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxIndividualNetworkScenariosGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve path parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve query parameters
	q := u.Query()
	log.Info("q: ", q)
	if q["network_scenario_id"] == nil || len(q["network_scenario_id"]) == 0 {
		err := errors.New("Wrong parameters: network_scenario_id")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	network_scenario_id := q["network_scenario_id"][0]
	log.Info("network_scenario_id: ", network_scenario_id)

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	scenario, resp, err := sandboxApiConnectors.pfmCtrlClient.ScenarioConfigurationApi.GetScenario(context.TODO(), network_scenario_id)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandboxIndividualNetworkScenariosGET: resp: ", resp)
	log.Info("sandboxIndividualNetworkScenariosGET: scenario: ", scenario)

	l := make([]platformCtrlClient.Scenario, 1)
	l[0] = scenario

	// Marshalling
	jsonResponse, err := json.Marshal(l)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("jsonResponse: ", string(jsonResponse))

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func sandboxNetworkScenarioPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxNetworkScenarioPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve path parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve query parameters
	q := u.Query()
	log.Info("q: ", q)
	if q["network_scenario_id"] == nil || len(q["network_scenario_id"]) == 0 {
		err := errors.New("Wrong parameters: network_scenario_id")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	network_scenario_id := q["network_scenario_id"][0]
	log.Info("network_scenario_id: ", network_scenario_id)

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	_, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].ActiveScenarioApi.ActivateScenario(context.TODO(), network_scenario_id, nil)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := applyGisAutomationAfterActivate(sandbox_name); err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	deleteUesIfAny(sandbox_name)

	w.WriteHeader(http.StatusNoContent)
}

func sandboxNetworkScenarioDELETE(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxNetworkScenarioDELETE: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve path parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	_, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].ActiveScenarioApi.TerminateScenario(context.TODO())
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	deleteUesIfAny(sandbox_name)

	w.WriteHeader(http.StatusNoContent)
}

func sandboxMecServicesGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxMecServicesGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve query parameters
	log.Info("q: ", u.Query())

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, err.Error())
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, err.Error())
		return
	}

	services, _, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].ServicesApi.ServicesGET(context.TODO(), nil)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, err.Error())
		return
	}

	// Marshalling
	jsonResponse, err := json.Marshal(services)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Info("jsonResponse: ", string(jsonResponse))

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func sandboxAppInstancesGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxAppInstancesGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve query parameters
	log.Info("q: ", u.Query())

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	appInstancesList, resp, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].ApplicationsApi.ApplicationsGET(context.TODO(), nil)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandboxAppInstancesGET: resp: ", resp)
	log.Info("sandboxAppInstancesGET: appInstancesList: ", appInstancesList)

	// Marshalling
	jsonResponse, err := json.Marshal(appInstancesList)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("jsonResponse: ", string(jsonResponse))

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func sandboxAppInstancesPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxAppInstancesPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	var applicationInfo ApplicationInfo
	log.Info("sandboxAppInstancesPOST: r.Body: ", r.Body)
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	err := json.Unmarshal(bodyBytes, &applicationInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("sandboxAppInstancesPOST: bodyBytes: ", string(bodyBytes))

	appInfo := sandboxCtrlClient.ApplicationInfo{
		Id:       applicationInfo.Id,
		Name:     applicationInfo.Name,
		NodeName: applicationInfo.NodeName,
		Type_:    applicationInfo.Type_,
		Persist:  applicationInfo.Persist,
	}
	log.Info("sandboxAppInstancesPOST: appInfo: ", appInfo)
	appInfo, resp, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].ApplicationsApi.ApplicationsPOST(context.TODO(), appInfo)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, err.Error())
		return
	}
	log.Info("sandboxAppInstancesPOST: resp: ", resp)
	log.Info("sandboxAppInstancesPOST: appInfo: ", appInfo)

	jsonResponse, err := json.Marshal(appInfo)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, err.Error())
		return
	}
	log.Info("sandboxAppInstancesPOST: jsonResponse: ", string(jsonResponse))

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, string(jsonResponse))
}

func sandboxAppInstancesDELETE(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxAppInstancesDELETE: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	appInstanceId := vars["app_instance_id"]
	if appInstanceId == "" {
		err := errors.New("Wrong parameters: app_instance_id")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("app_instance_id: ", appInstanceId)

	_, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].ApplicationsApi.ApplicationsAppInstanceIdDELETE(context.TODO(), appInstanceId)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func sandboxUeControllerGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxUeControllerGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name] == nil {
		err := errors.New("Sandbox not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, ok := Ues[sandbox_name]; !ok {
		// Get active scenario
		scenario, _, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].ActiveScenarioApi.GetActiveScenario(context.TODO(), nil)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ues, ues_count, err := parseUes(scenario)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		Ues[sandbox_name] = ues
		Ues_count[sandbox_name] = ues_count
		log.Info("Ues: ", Ues[sandbox_name])
		log.Info("Ues_count: ", Ues_count[sandbox_name])

		if len(Ues) != len(Ues_count) {
			err := errors.New("Invalid UEs maps length")
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	result := make([]Ue, len(Ues[sandbox_name]))
	i := 0
	for k := range Ues[sandbox_name] {
		result[i] = Ue{Id: k, Count: Ues_count[sandbox_name][k]}
		i += 1
	}

	// Marshalling
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Info("jsonResponse: ", string(jsonResponse))

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func sandboxUeControllerPATCH(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sandboxUeControllerPATCH: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	// Retrieve query parameters
	q := u.Query()
	log.Info("q: ", q)
	if q["user_equipment_id"] == nil || q["user_equipment_value"] == nil || len(q["user_equipment_id"]) == 0 || len(q["user_equipment_value"]) == 0 {
		err := errors.New("Wrong parameters")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	user_equipment_id := q["user_equipment_id"][0]
	log.Info("user_equipment_id: ", user_equipment_id)
	user_equipment_value, _ := strconv.Atoi(q["user_equipment_value"][0])
	log.Info("user_equipment_value: ", user_equipment_value)

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	sandbox_name := vars["sandbox_name"]
	if sandbox_name == "" {
		err := errors.New("Wrong parameters: sandbox_name")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("sandbox_name: ", sandbox_name)

	if _, ok := Ues[sandbox_name][user_equipment_id]; !ok {
		err := errors.New("Sandbox or UE not found in scenario")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	v := Ues[sandbox_name][user_equipment_id]
	log.Info("v: ", v)
	log.Info("len(v): ", (v))
	log.Info("v.Name: ", v[user_equipment_value].Name)
	log.Info("v.UserMeta: ", v[user_equipment_value].UserMeta)

	if user_equipment_value >= len(v) {
		err := errors.New("Invalid increment/decrement value")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	var eventScenarioUpdate sandboxCtrlClient.EventScenarioUpdate
	log.Info("Ues_count["+sandbox_name+"]["+user_equipment_id+"]: ", Ues_count[sandbox_name][user_equipment_id])
	log.Info("user_equipment_value: ", user_equipment_value)
	if Ues_count[sandbox_name][user_equipment_id] > int32(user_equipment_value) { // Decrease
		eventScenarioUpdate.Action = "REMOVE"
		Ues_count[sandbox_name][user_equipment_id] -= 1
	} else {
		eventScenarioUpdate.Action = "ADD"
		Ues_count[sandbox_name][user_equipment_id] += 1
	}
	log.Info("eventScenarioUpdate.Action: ", eventScenarioUpdate.Action)
	s := strconv.Itoa(user_equipment_value)
	Ues[sandbox_name][user_equipment_id][user_equipment_value].UserMeta["NumInstance"] = s
	log.Info("New value for ", user_equipment_id, " is ", Ues[sandbox_name][user_equipment_id][user_equipment_value].UserMeta["NumInstance"])

	var physicalLocation = sandboxCtrlClient.PhysicalLocation{
		Name:         v[user_equipment_value].Name,
		Type_:        v[user_equipment_value].Type_,
		GeoData:      v[user_equipment_value].GeoData,
		Meta:         v[user_equipment_value].Meta,
		Connected:    v[user_equipment_value].Connected,
		Wireless:     v[user_equipment_value].Wireless,
		WirelessType: v[user_equipment_value].WirelessType,
		NetChar:      v[user_equipment_value].NetChar,
		MacId:        v[user_equipment_value].MacId,
	}
	var un = sandboxCtrlClient.NodeDataUnion{
		PhysicalLocation: &physicalLocation,
	}
	var node = sandboxCtrlClient.ScenarioNode{
		Type_:         "UE",
		NodeDataUnion: &un,
		Parent:        v[user_equipment_value].UserMeta["ParentName"],
	}
	eventScenarioUpdate.Node = &node
	var event = sandboxCtrlClient.Event{
		Name:                v[user_equipment_value].Name,
		Type_:               "SCENARIO-UPDATE",
		EventScenarioUpdate: &eventScenarioUpdate,
	}
	log.Info("event:", event)
	_, err := sandboxApiConnectors.sandboxCtrlAppClient[sandbox_name].EventsApi.SendEvent(context.TODO(), event.Type_, event)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Retrieve existing user session or create a new one
func startSession(provider string, username string, w http.ResponseWriter, r *http.Request, createSandbox bool) (sandboxName string, isNew bool, userRole string, err error, code int) {
	log.Debug(">>> startSession")
	log.Info(r)
	log.Info("provider: ", provider)
	log.Info("username: ", username)

	// Get existing session by user name, if any
	sessionStore := sandboxApiConnectors.sessionMgr.GetSessionStore()
	session, err := sessionStore.GetByName(provider, username)
	log.Info("sessionStore: ", sessionStore)
	log.Info("session: ", session)
	log.Info("session: ", fmt.Sprintf("session: %v", session))
	if err != nil {
		// Get requested sandbox name & role from user profile, if any
		providerMode := "open"
		log.Info("providerMode: ", providerMode)
		role := users.RoleUser
		log.Info("role: ", role)
		user, err := sandboxApiConnectors.userStore.GetUser(provider, username)
		if err != nil {
			log.Error(err.Error())
		}
		log.Info("user: ", user)
		// Create sandbox
		if createSandbox {
			var sandboxConfig platformCtrlClient.SandboxConfig
			if sandboxName == "" {
				sandbox, _, err := sandboxApiConnectors.pfmCtrlClient.SandboxControlApi.CreateSandbox(context.TODO(), sandboxConfig)
				if err != nil {
					log.Error(err.Error())
					return "", false, "", err, http.StatusInternalServerError
				}
				sandboxName = sandbox.Name
			} else {
				_, err := sandboxApiConnectors.pfmCtrlClient.SandboxControlApi.CreateSandboxWithName(context.TODO(), sandboxName, sandboxConfig)
				if err != nil {
					log.Error(err.Error())
					return "", false, "", err, http.StatusInternalServerError
				}
			}
			// Create new user & store sandbox name for next use
			if user == nil {
				err = sandboxApiConnectors.userStore.CreateUser(provider, username, "", role, sandboxName)
				if err != nil {
					log.Error("Failed to create user with err: ", err.Error())
				}
			}
		}
		// Create new session
		session = new(sm.Session)
		session.ID = ""
		session.Username = username
		session.Provider = provider
		session.Sandbox = sandboxName
		session.Role = role
		isNew = true
	} else {
		sandboxName = session.Sandbox
	}
	userRole = session.Role
	log.Info("session: ", session)

	// Set session
	sandboxApiConnectors.sessionStore[sandboxName] = session
	// err, code = sessionStore.Set(session, w, r)
	// if err != nil {
	// 	log.Error("Failed to set session with err: ", err.Error())
	// 	// Remove newly created sandbox on failure
	// 	if session.ID == "" && createSandbox {
	// 		_, _ = sandboxApiConnectors.pfmCtrlClient.SandboxControlApi.DeleteSandbox(context.TODO(), sandboxName)
	// 	}
	// 	return "", false, "", err, code
	// }
	log.Info("code: ", code)
	log.Info("sandboxName: ", sandboxName)
	log.Info("isNew: ", isNew)

	return sandboxName, isNew, userRole, nil, http.StatusOK
}

func processLogout(sandboxName string, w http.ResponseWriter, r *http.Request) error {
	log.Debug(">>> processLogout: ", sandboxName)

	session := sandboxApiConnectors.sessionStore[sandboxName]
	delete(sandboxApiConnectors.sessionStore, sandboxName)
	delete(sandboxApiConnectors.sandboxCtrlAppClient, sandboxName)

	var metric met.SessionMetric
	sandboxDeleted := false
	metricSessionLogout.Inc()

	metric.Provider = session.Provider
	metric.User = session.Username
	metric.Sandbox = session.Sandbox

	// Delete sandbox
	if session.Sandbox != "" {
		_, err := sandboxApiConnectors.pfmCtrlClient.SandboxControlApi.DeleteSandbox(context.TODO(), session.Sandbox)
		if err == nil {
			log.Info("Sandbox deleted")
			sandboxDeleted = true
		}
	}

	_ = sandboxApiConnectors.metricStore.SetSessionMetric(met.SesMetTypeLogout, metric)
	if sandboxDeleted {
		metricSessionActive.Dec()
		metricSessionDuration.Observe(time.Since(session.StartTime).Minutes())
	}

	return nil
}

func sessionTimeoutCb(session *sm.Session) {
	log.Info("Session timed out. ID[", session.ID, "] Username[", session.Username, "]")

	delete(sandboxApiConnectors.sandboxCtrlAppClient, session.Sandbox)

	var metric met.SessionMetric
	metric.Provider = session.Provider
	metric.User = session.Username
	metric.Sandbox = session.Sandbox
	_ = sandboxApiConnectors.metricStore.SetSessionMetric(met.SesMetTypeTimeout, metric)
	metricSessionTimeout.Inc()
	// Destroy session sandbox
	if session.Sandbox != "" {
		_, err := sandboxApiConnectors.pfmCtrlClient.SandboxControlApi.DeleteSandbox(context.TODO(), session.Sandbox)
		if err == nil {
			metricSessionActive.Dec()
			metricSessionDuration.Observe(time.Since(session.StartTime).Minutes())
		}
	}
}

func deleteUesIfAny(sandbox_name string) {
	if _, ok := Ues[sandbox_name]; ok {
		delete(Ues, sandbox_name)
		delete(Ues_count, sandbox_name)
	}
}

func copyUePhysicalLocation(pl sandboxCtrlClient.PhysicalLocation, userMeta map[string]string) sandboxCtrlClient.PhysicalLocation {
	return sandboxCtrlClient.PhysicalLocation{
		Name:         pl.Name,
		Type_:        pl.Type_,
		GeoData:      pl.GeoData,
		Meta:         pl.Meta,
		UserMeta:     userMeta,
		Connected:    pl.Connected,
		Wireless:     pl.Wireless,
		WirelessType: pl.WirelessType,
		NetChar:      pl.NetChar,
		MacId:        pl.MacId,
	}
}

// ueSpeedClass: hv / lv / zv. SUMO UEs often have velocity 0 (no GIS path);
// classify by name or map icon so monaco-harbor cars/peds are not all stationary.
func ueSpeedClass(pl sandboxCtrlClient.PhysicalLocation, velocityThreshold int) string {
	if pl.GeoData != nil && pl.GeoData.Velocity != 0 {
		if pl.GeoData.Velocity >= float32(velocityThreshold) {
			return "hv"
		}
		return "lv"
	}
	name := strings.ToLower(pl.Name)
	icon := ""
	if pl.Meta != nil {
		icon = pl.Meta["display.map.icon"]
	}
	if strings.HasPrefix(name, "ue-car") || strings.Contains(icon, "car") {
		return "hv"
	}
	if strings.HasPrefix(name, "ue-ped") || strings.Contains(icon, "walk") {
		return "lv"
	}
	return "zv"
}

func parseUes(scenario sandboxCtrlClient.Scenario) (ues map[string][]sandboxCtrlClient.PhysicalLocation, ues_count map[string]int32, err error) {
	log.Debug(">>> parseUes: ", scenario)

	var velocityThreshold = 10 //DEFAULT_VELOCITY_THRESHOLD

	// Get initial UE counts from scenario meta data
	if scenario.Deployment.UserMeta != nil {
		mecSandboxUserMeta := scenario.Deployment.UserMeta["mec-sandbox"]
		if mecSandboxUserMeta != "" {
			// Mixed types (connectivityModel string + count ints); do not Unmarshal into map[string]int.
			var jsonUeObj map[string]interface{}
			if uerr := json.Unmarshal([]byte(mecSandboxUserMeta), &jsonUeObj); uerr != nil {
				log.Error("parseUes: mec-sandbox userMeta: ", uerr.Error())
			} else if raw, ok := jsonUeObj["highVelocitySpeedThreshold"]; ok {
				switch n := raw.(type) {
				case float64:
					velocityThreshold = int(n)
				case int:
					velocityThreshold = n
				}
			}
		}
	}
	log.Info("parseUes: velocityThreshold: ", velocityThreshold)

	// Get all UEs from scenario
	var zvList = make([]sandboxCtrlClient.PhysicalLocation, 0)
	var lvList = make([]sandboxCtrlClient.PhysicalLocation, 0)
	var hvList = make([]sandboxCtrlClient.PhysicalLocation, 0)

	for i := range scenario.Deployment.Domains {
		domain := &scenario.Deployment.Domains[i]

		for j := range domain.Zones {
			zone := domain.Zones[j]

			for k := range zone.NetworkLocations {
				nl := zone.NetworkLocations[k] // Network Locations
				var userMeta = map[string]string{
					"ParentName": nl.Name,
				}

				for l := range nl.PhysicalLocations {
					pl := nl.PhysicalLocations[l] // Physical Locations

					if pl.Type_ == "UE" && pl.GeoData != nil {
						entry := copyUePhysicalLocation(pl, userMeta)
						switch ueSpeedClass(pl, velocityThreshold) {
						case "hv":
							hvList = append(hvList, entry)
						case "lv":
							lvList = append(lvList, entry)
						default:
							zvList = append(zvList, entry)
						}
					}
				} // End of 'for'statement
			} // End of 'for'statement
		} // End of 'for'statement
	} // End of 'for'statement
	log.Info("parseUes: zvList: ", zvList)
	log.Info("parseUes: hvList: ", hvList)
	log.Info("parseUes: lvList: ", lvList)
	ues = map[string][]sandboxCtrlClient.PhysicalLocation{
		"defaultStaticUeCount":       zvList,
		"defaultLowVelocityUeCount":  lvList,
		"highVelocitySpeedThreshold": hvList,
	}
	ues_count = map[string]int32{
		"defaultStaticUeCount":       int32(len(zvList)),
		"defaultLowVelocityUeCount":  int32(len(lvList)),
		"highVelocitySpeedThreshold": int32(len(hvList)),
	}

	return ues, ues_count, nil
}

func usesExternalNwSimulator() bool {
	t := strings.ToLower(strings.TrimSpace(os.Getenv("MEEP_NW_SIMULATOR_TYPE")))
	return t == "simu5g" || t == "oai"
}

func applyGisAutomationAfterActivate(sandboxName string) error {
	gisClient := sandboxApiConnectors.gisClient[sandboxName]
	if gisClient == nil {
		return errors.New("GIS client not found")
	}
	movement := true
	poasInRange := false
	if usesExternalNwSimulator() {
		movement = false
		poasInRange = true
	}
	if _, err := gisClient.AutomationApi.SetAutomationStateByName(context.TODO(), "MOVEMENT", movement); err != nil {
		return err
	}
	if _, err := gisClient.AutomationApi.SetAutomationStateByName(context.TODO(), "MOBILITY", true); err != nil {
		return err
	}
	if usesExternalNwSimulator() {
		if _, err := gisClient.AutomationApi.SetAutomationStateByName(context.TODO(), "NETWORK-CHARACTERISTICS-UPDATE", false); err != nil {
			return err
		}
	}
	if poasInRange {
		if _, err := gisClient.AutomationApi.SetAutomationStateByName(context.TODO(), "POAS-IN-RANGE", true); err != nil {
			return err
		}
	}
	return nil
}
