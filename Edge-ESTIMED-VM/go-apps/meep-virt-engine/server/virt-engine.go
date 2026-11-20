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

package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-virt-engine/helm"
	dataModel "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-model"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	wd "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-watchdog"
)

const redisAddr string = "meep-redis-master:6379"

// const retryTimerDuration = 10000

const moduleName string = "meep-virt-engine"
const moduleNamespace string = "default"

// MQ payload fields
const fieldSandboxName = "sandbox-name"
const fieldScenarioName = "scenario-name"
const fieldEventType = "event-type"
const fieldNodeName = "node-name"

const appEnablementSvcName = "meep-app-enablement"

type VirtEngine struct {
	wdPinger            *wd.Pinger
	mqGlobal            *mq.MsgQueue
	activeModels        map[string]*mod.Model
	activeScenarioNames map[string]string
	hostUrl             string
	userSwagger         string
	httpsOnly           bool
	authEnabled         bool
	handlerId           int
	sboxPods            map[string]string
	sboxChartTemplates  map[string]bool
	globalAppEnablement bool
}

var ve *VirtEngine

// Init - Initialize virtualization engine
func Init() (err error) {
	log.Debug("Initializing MEEP Virtualization Engine")
	ve = new(VirtEngine)
	ve.activeModels = make(map[string]*mod.Model)
	ve.activeScenarioNames = make(map[string]string)

	// Retrieve list of available sandbox chart templates
	templates, err := getSanboxChartTemplates()
	if err != nil {
		log.Error(err.Error())
		return err
	}
	ve.sboxChartTemplates = make(map[string]bool)
	for _, t := range templates {
		ve.sboxChartTemplates[t] = true
	}

	// Retrieve sandbox pods list from environment variable
	ve.sboxPods = make(map[string]string)
	sboxPodsStr := strings.TrimSpace(os.Getenv("MEEP_SANDBOX_PODS"))
	log.Info("MEEP_SANDBOX_PODS: ", sboxPodsStr)
	if sboxPodsStr != "" {
		sboxPodsList := strings.Split(sboxPodsStr, ",")
		for _, pod := range sboxPodsList {
			ve.sboxPods[pod] = pod

			// Check if App Enablement global service is present
			if pod == appEnablementSvcName {
				ve.globalAppEnablement = true
			}
		}
	}
	if len(ve.sboxPods) == 0 {
		err = errors.New("MEEP_SANDBOX_PODS env variable does not contain sbox pod list")
		log.Error(err.Error())
		return err
	}

	// Retrieve Host Name from environment variable
	ve.hostUrl = strings.TrimSpace(os.Getenv("MEEP_HOST_URL"))
	if ve.hostUrl == "" {
		err = errors.New("MEEP_HOST_URL env variable not set")
		log.Error(err.Error())
		return err
	}
	log.Info("MEEP_HOST_URL: ", ve.hostUrl)

	// Retrieve User Swagger from environment variable
	ve.userSwagger = strings.TrimSpace(os.Getenv("MEEP_USER_SWAGGER"))
	if ve.userSwagger == "" {
		err = errors.New("MEEP_USER_SWAGGER variable not set")
		log.Error(err.Error())
		return err
	}
	log.Info("MEEP_USER_SWAGGER: ", ve.userSwagger)

	// Retrieve HTTPS only mode from environment variable
	httpsOnlyStr := strings.TrimSpace(os.Getenv("MEEP_HTTPS_ONLY"))
	httpsOnly, err := strconv.ParseBool(httpsOnlyStr)
	if err == nil {
		ve.httpsOnly = httpsOnly
	}
	log.Info("MEEP_HTTPS_ONLY: ", httpsOnlyStr)

	// Retrieve Auth mode from environment variable
	authEnabledStr := strings.TrimSpace(os.Getenv("MEEP_AUTH_ENABLED"))
	authEnabled, err := strconv.ParseBool(authEnabledStr)
	if err == nil {
		ve.authEnabled = authEnabled
	}
	log.Info("MEEP_AUTH_ENABLED: ", authEnabledStr)

	// Create message queue
	ve.mqGlobal, err = mq.NewMsgQueue(mq.GetGlobalName(), moduleName, moduleNamespace, redisAddr)
	if err != nil {
		log.Error("Failed to create Message Queue with error: ", err)
		return err
	}
	log.Info("Message Queue created")

	// Setup for liveness monitoring
	ve.wdPinger, err = wd.NewPinger(moduleName, moduleNamespace, redisAddr)
	if err != nil {
		log.Error("Failed to initialize pinger. Error: ", err)
		return err
	}
	err = ve.wdPinger.Start()
	if err != nil {
		log.Error("Failed watchdog client listen. Error: ", err)
		return err
	}

	return nil
}

// Run - Start Virt Engine execution
func Run() (err error) {
	log.Debug("Starting MEEP Virtualization Engine")

	// Register Message Queue handler
	handler := mq.MsgHandler{Handler: msgHandler, UserData: nil}
	ve.handlerId, err = ve.mqGlobal.RegisterHandler(handler)
	if err != nil {
		log.Error("Failed to listen for sandbox updates: ", err.Error())
		return err
	}

	// Start REST API Server
	router := mux.NewRouter()
	router.HandleFunc("/virt-engine/v1/onboard_app", onboardAppHandler).Methods("POST")
	router.HandleFunc("/virt-engine/v1/onboard_app", getOnboardedAppsHandler).Methods("GET")

	go func() {
		log.Info("Starting virt-engine REST API on port 80")
		if err := http.ListenAndServe(":80", router); err != nil {
			log.Error("Failed to start REST API: ", err)
		}
	}()

	return nil
}

// Dynamic Onboarded apps (DAI) - Experimental
func onboardAppHandler(w http.ResponseWriter, r *http.Request) {
	log.Info("Received POST /onboard_app request")

	// Parse multipart form (max 50MB)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
		return
	}

	// 1. Get the AppD JSON
	appDStr := r.FormValue("appD")
	if appDStr == "" {
		http.Error(w, "Missing appD parameter", http.StatusBadRequest)
		return
	}

	var appD map[string]interface{}
	if err := json.Unmarshal([]byte(appDStr), &appD); err != nil {
		http.Error(w, "Invalid appD JSON", http.StatusBadRequest)
		return
	}

	// Extract appDId to use as the file/chart name
	appDId, ok := appD["appDId"].(string)
	if !ok || appDId == "" {
		http.Error(w, "appDId is missing in appD JSON", http.StatusBadRequest)
		return
	}

	// 2. Get the Helm Chart
	file, _, err := r.FormFile("helmChart")
	if err != nil {
		http.Error(w, "Missing helmChart file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Ensure app-catalog directory exists
	userChartsDir := "/data/app-catalog"
	if err := os.MkdirAll(userChartsDir, os.ModePerm); err != nil {
		log.Error("Failed to create user-charts directory: ", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Save the Helm chart
	chartPath := filepath.Join(userChartsDir, appDId+".tgz")
	dest, err := os.Create(chartPath)
	if err != nil {
		log.Error("Failed to create helm chart file: ", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		log.Error("Failed to save helm chart: ", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	log.Info("Successfully saved Helm chart to ", chartPath)

	// 3. Save AppD to local catalog file
	catalogFile := filepath.Join(userChartsDir, "app_catalog.json")
	var catalog []map[string]interface{}

	// Read existing catalog if it exists
	if b, err := os.ReadFile(catalogFile); err == nil {
		json.Unmarshal(b, &catalog)
	}

	// Append the new AppD
	catalog = append(catalog, appD)

	// Write back
	if b, err := json.MarshalIndent(catalog, "", "  "); err == nil {
		os.WriteFile(catalogFile, b, 0644)
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Application onboarded successfully"))
}

func getOnboardedAppsHandler(w http.ResponseWriter, r *http.Request) {
	log.Info("Received GET /onboard_app request")
	catalogFile := "/data/app-catalog/app_catalog.json"

	b, err := os.ReadFile(catalogFile)
	if err != nil {
		b = []byte("[]") // Return empty array if file not found
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

// Message Queue handler
func msgHandler(msg *mq.Msg, userData interface{}) {
	switch msg.Message {
	case mq.MsgSandboxCreate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		createSandbox(msg.Payload[fieldSandboxName])
	case mq.MsgSandboxDestroy:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		destroySandbox(msg.Payload[fieldSandboxName])
	case mq.MsgScenarioActivate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		activateScenario(msg.Payload[fieldSandboxName])
	case mq.MsgScenarioUpdate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		eventType := msg.Payload[fieldEventType]
		sandboxName := msg.Payload[fieldSandboxName]
		nodeName := msg.Payload[fieldNodeName]
		switch eventType {
		case mod.EventAddNode:
			addScenarioNode(sandboxName, nodeName)
		case mod.EventModifyNode:
			modifyScenarioNode(sandboxName, nodeName)
		case mod.EventRemoveNode:
			removeScenarioNode(sandboxName, nodeName)
		default:
			log.Trace("Ignoring unsupported scenario update event type: ", eventType)
		}
	case mq.MsgScenarioTerminate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		terminateScenario(msg.Payload[fieldSandboxName], msg.Payload[fieldScenarioName])
	default:
		log.Trace("Ignoring unsupported message: ", mq.PrintMsg(msg))
	}
}

func getModel(sandboxName string) *mod.Model {
	activeModel := ve.activeModels[sandboxName]
	if activeModel == nil {
		modelCfg := mod.ModelCfg{
			Name:      moduleName,
			Namespace: sandboxName,
			Module:    moduleName,
			DbAddr:    redisAddr,
			UpdateCb:  nil,
		}
		var err error
		activeModel, err = mod.NewModel(modelCfg)
		if err != nil {
			log.Error("Failed to create model: ", err.Error())
			return nil
		}
		ve.activeModels[sandboxName] = activeModel
	}
	return activeModel
}

func activateScenario(sandboxName string) {
	// Get sandbox-specific active model
	activeModel := getModel(sandboxName)
	if activeModel == nil {
		log.Error("No active model for sandbox: ", sandboxName)
		return
	}

	// Sync with active scenario store
	activeModel.UpdateScenario()

	// Cache name for later deletion
	ve.activeScenarioNames[sandboxName] = activeModel.GetScenarioName()

	// Deploy scenario
	err := Deploy(sandboxName, "", activeModel)
	if err != nil {
		log.Error("Error creating charts: ", err)
		return
	}
}

func addScenarioNode(sandboxName string, nodeName string) {
	log.Info("Adding node: ", nodeName)

	// Get sandbox-specific active model
	activeModel := getModel(sandboxName)
	if activeModel == nil {
		log.Error("No active model for sandbox: ", sandboxName)
		return
	}

	// Sync with active scenario store
	activeModel.UpdateScenario()

	// Deploy single process
	nodeType := activeModel.GetNodeType(nodeName)
	if mod.IsProc(nodeType) {
		err := Deploy(sandboxName, nodeName, activeModel)
		if err != nil {
			log.Error("Error creating charts: ", err)
			return
		}
	} else if mod.IsPhyLoc(nodeType) {
		node := activeModel.GetNode(nodeName)
		pl, ok := node.(*dataModel.PhysicalLocation)
		if !ok {
			log.Error("Error casting physical location: " + nodeName)
			return
		}
		for _, proc := range pl.Processes {
			err := Deploy(sandboxName, proc.Name, activeModel)
			if err != nil {
				log.Error("Error creating charts for process ", proc.Name, ": ", err)
				continue
			}
		}
	} else {
		log.Error("Unsupported node type: ", nodeType)
		return
	}
}

func modifyScenarioNode(sandboxName string, nodeName string) {
	log.Info("Modifying node: ", nodeName)

	// Get sandbox-specific active model
	activeModel := getModel(sandboxName)
	if activeModel == nil {
		log.Error("No active model for sandbox: ", sandboxName)
		return
	}

	// Get cached scenario name
	scenarioName := ve.activeScenarioNames[sandboxName]
	if scenarioName == "" {
		scenarioName = activeModel.GetScenarioName()
		ve.activeScenarioNames[sandboxName] = scenarioName
	}

	// Sync with active scenario store
	activeModel.UpdateScenario()

	// Handle process updates only
	nodeType := activeModel.GetNodeType(nodeName)
	if mod.IsProc(nodeType) {

		// Remove existing process
		_, chartsToDelete := deleteReleases(sandboxName, scenarioName, nodeName)
		log.Info("Number of charts to be deleted: ", chartsToDelete)

		// Add updated process
		err := Deploy(sandboxName, nodeName, activeModel)
		if err != nil {
			log.Error("Error creating charts: ", err)
			return
		}
	} else {
		log.Error("Unsupported node type: ", nodeType)
		return
	}
}

func removeScenarioNode(sandboxName string, nodeName string) {
	log.Info("Removing node: ", nodeName)
	if nodeName == "" {
		log.Error("Missing node name")
		return
	}

	// Get sandbox-specific active model
	activeModel := getModel(sandboxName)
	if activeModel == nil {
		log.Error("No active model for sandbox: ", sandboxName)
		return
	}

	// Get cached scenario name
	scenarioName := ve.activeScenarioNames[sandboxName]
	if scenarioName == "" {
		scenarioName = activeModel.GetScenarioName()
		ve.activeScenarioNames[sandboxName] = scenarioName
	}

	// Before updating active scenario, find processes to remove
	procNames := []string{}
	node := activeModel.GetNode(nodeName)
	nodeType := activeModel.GetNodeType(nodeName)
	if mod.IsPhyLoc(nodeType) {
		pl, ok := node.(*dataModel.PhysicalLocation)
		if !ok {
			log.Error("Error casting physical location: " + nodeName)
			return
		}
		for _, proc := range pl.Processes {
			procNames = append(procNames, proc.Name)
		}
	} else if mod.IsProc(nodeType) {
		proc, ok := node.(*dataModel.Process)
		if !ok {
			log.Error("Error casting process: " + nodeName)
			return
		}
		procNames = append(procNames, proc.Name)
	} else {
		log.Error("Unsupported node type: ", nodeType)
		return
	}

	// Sync with active scenario store
	activeModel.UpdateScenario()

	// Delete releases & remove charts for each process
	for _, procName := range procNames {
		_, chartsToDelete := deleteReleases(sandboxName, scenarioName, procName)
		log.Info("Number of charts to be deleted: ", chartsToDelete)
	}
}

func terminateScenario(sandboxName string, scenarioName string) {
	if sandboxName == "" {
		return
	}

	log.Info("Terminating scenario for sandbox: ", sandboxName)

	if scenarioName == "" {
		// Get cached scenario name
		scenarioName = ve.activeScenarioNames[sandboxName]
		if scenarioName == "" && ve.activeModels[sandboxName] != nil {
			scenarioName = ve.activeModels[sandboxName].GetScenarioName()
		}
	}

	if scenarioName == "" {
		log.Info("No active scenario to terminate for sandbox: ", sandboxName)
		return
	}

	// Process right away and start a ticker to retry until everything is deleted
	_, chartsToDelete := deleteReleases(sandboxName, scenarioName, "")
	log.Info("Number of charts to be deleted: ", chartsToDelete)
	ve.activeScenarioNames[sandboxName] = ""

	// Clean up any leftover cluster role bindings (do not delete sandbox pod bindings)
	cleanUpClusterRoleBindings(sandboxName, false)

	// ticker := time.NewTicker(retryTimerDuration * time.Millisecond)

	// go func() {
	// 	for range ticker.C {
	// 		err, chartsToDelete := deleteReleases(sandboxName, scenarioName)
	// 		if err == nil && chartsToDelete == 0 {
	// 			// Remove model & cached scenario
	// 			ve.activeScenarioNames[sandboxName] = ""
	// 			ticker.Stop()
	// 			return
	// 		} else {
	// 			// Stay in the deletion process until everything is cleared
	// 			log.Info("Number of charts remaining to be deleted: ", chartsToDelete)
	// 		}
	// 	}
	// }()
}

func createSandbox(sandboxName string) {
	var err error

	// Clean up any leftover cluster role bindings first (clean all including old sandbox pod bindings)
	cleanUpClusterRoleBindings(sandboxName, true)

	// Create new Model instance
	modelCfg := mod.ModelCfg{
		Name:      moduleName,
		Namespace: sandboxName,
		Module:    moduleName,
		DbAddr:    redisAddr,
		UpdateCb:  nil,
	}
	ve.activeModels[sandboxName], err = mod.NewModel(modelCfg)
	if err != nil {
		log.Error("Failed to create model: ", err.Error())
		return
	}

	// Deploy sandbox
	err = deploySandbox(sandboxName)
	if err != nil {
		log.Error("Error deploying sandbox: ", err)
		return
	}
}

func destroySandbox(sandboxName string) {
	// Process right away and start a ticker to retry until everything is deleted
	_, chartsToDelete := deleteReleases(sandboxName, "", "")
	log.Info("Number of charts to be deleted: ", chartsToDelete)
	ve.activeScenarioNames[sandboxName] = ""
	ve.activeModels[sandboxName] = nil

	// Clean up any leftover cluster role bindings (clean all when destroying sandbox)
	cleanUpClusterRoleBindings(sandboxName, true)

	// ticker := time.NewTicker(retryTimerDuration * time.Millisecond)

	// go func() {
	// 	for range ticker.C {
	// 		err, chartsToDelete := deleteReleases(sandboxName, "")
	// 		if err == nil && chartsToDelete == 0 {
	// 			// Remove modle & cached scenario
	// 			ve.activeScenarioNames[sandboxName] = ""
	// 			ve.activeModels[sandboxName] = nil
	// 			ticker.Stop()
	// 			return
	// 		} else {
	// 			// Stay in the deletion process until everything is cleared
	// 			log.Info("Number of charts remaining to be deleted: ", chartsToDelete)
	// 		}
	// 	}
	// }()
}

func deleteReleases(sandboxName string, scenarioName string, procName string) (error, int) {
	if sandboxName == "" {
		return nil, 0
	}

	// Get chart prefix & path
	path := "/charts/" + sandboxName
	releasePrefix := "meep-"
	if scenarioName != "" {
		path += "/scenario/" + scenarioName
		releasePrefix += scenarioName + "-"
	}
	if procName != "" {
		path += "/" + procName
		releasePrefix += procName
	}

	// Retrieve list of releases
	chartsToDelete := 0
	rels, err := helm.GetReleasesName(sandboxName)
	if err == nil {
		// Filter charts by sandbox & scenario names
		var toDelete []helm.Chart
		for _, rel := range rels {
			if strings.HasPrefix(rel.Name, releasePrefix) {
				var c helm.Chart
				c.ReleaseName = rel.Name
				c.Namespace = sandboxName
				toDelete = append(toDelete, c)
			}
		}

		// Delete releases
		chartsToDelete = len(toDelete)
		if chartsToDelete > 0 {
			log.Debug("Deleting [", chartsToDelete, "] charts with release prefix: ", releasePrefix)
			err := helm.DeleteReleases(toDelete, sandboxName)
			chartsToDelete = len(toDelete)
			if err != nil {
				log.Debug("Releases deletion failure:", err)
			}
		}

		// Then delete charts (queued sequentially in Helm worker)
		if _, err := os.Stat(path); err == nil {
			log.Debug("Queueing chart removal from path: ", path)
			_ = helm.CleanChartDir(path)
		}
	}
	return err, chartsToDelete
}

func cleanUpClusterRoleBindings(sandboxName string, cleanAll bool) {
	log.Info("Cleaning up ClusterRoleBindings for sandbox: ", sandboxName, " (cleanAll=", cleanAll, ")")
	cmd := exec.Command("kubectl", "get", "clusterrolebindings", "-o", "name")
	out, err := cmd.Output()
	if err != nil {
		log.Error("Failed to get clusterrolebindings: ", err.Error())
		return
	}

	sboxPods := strings.Split(strings.TrimSpace(os.Getenv("MEEP_SANDBOX_PODS")), ",")
	sboxPodMap := make(map[string]bool)
	for _, pod := range sboxPods {
		sboxPodMap[strings.TrimSpace(pod)] = true
	}

	lines := strings.Split(string(out), "\n")
	prefix := "clusterrolebinding.rbac.authorization.k8s.io/" + sandboxName + ":"
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			if !cleanAll {
				podName := strings.TrimPrefix(line, prefix)
				if sboxPodMap[podName] {
					continue
				}
			}
			log.Info("Deleting leftover clusterrolebinding: ", line)
			deleteCmd := exec.Command("kubectl", "delete", line)
			_ = deleteCmd.Run()
		}
	}
}
