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
	//mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"

	"github.com/gorilla/mux"
)

const redisTestAddr = "localhost:30380"
const influxTestAddr = "http://localhost:30986"
const testScenarioName = "testScenario"

const (
	postgresTestHost = "localhost"
	postgresTestPort = "30432"
)

func TestLoginPost(t *testing.T) {
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

	rr, err := sendRequest(http.MethodPost, "/login?provider=github", nil, nil, nil, nil, http.StatusCreated, Login)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	var respBody Sandbox
	err = json.Unmarshal([]byte(rr), &respBody)
	if err != nil {
		t.Fatalf(err.Error())
	}

	if respBody.Name == "" {
		err = errors.New("Invalid sandbox name")
		t.Fatalf(err.Error())
	}

	time.Sleep(5 * time.Second)

	_, err = sendRequest(http.MethodPost, "/logout?sandbox_name="+respBody.Name, nil, nil, nil, nil, http.StatusOK, Logout)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")

	time.Sleep(5 * time.Second)
}

func TestSandboxNetworkScenariosGET(t *testing.T) {
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

	sandboxName, err := testLoginPost()
	if err != nil {
		t.Fatalf(err.Error())
	}

	vars := make(map[string]string)
	vars["sandbox_name"] = sandboxName
	rr, err := sendRequest(http.MethodGet, "/sandboxNetworkScenarios", nil, vars, nil, nil, http.StatusOK, SandboxNetworkScenariosGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")
	fmt.Println("rr: ", rr)

	var respBody []SandboxNetworkScenario
	err = json.Unmarshal([]byte(rr), &respBody)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("respBody: ", respBody)

	_ = testLogoutPost(sandboxName)
}

func TestSandboxIndividualNetworkScenariosGET(t *testing.T) {
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

	sandboxName, err := testLoginPost()
	if err != nil {
		t.Fatalf(err.Error())
	}

	vars := make(map[string]string)
	vars["sandbox_name"] = sandboxName
	rr, err := sendRequest(http.MethodGet, "/sandboxNetworkScenarios/4g-5g-macro-v2x", nil, vars, nil, nil, http.StatusOK, SandboxIndividualNetworkScenariosGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")
	fmt.Println("rr: ", rr)

	_ = testLogoutPost(sandboxName)
}

func TestSandboxAppInstancesGET(t *testing.T) {
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

	sandboxName, err := testLoginPost()
	if err != nil {
		t.Fatalf(err.Error())
	}

	vars := make(map[string]string)
	vars["sandbox_name"] = sandboxName
	rr, err := sendRequest(http.MethodGet, "/sandboxAppInstances", nil, vars, nil, nil, http.StatusOK, SandboxAppInstancesGET)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println("Request done")
	fmt.Println("rr: ", rr)

	_ = testLogoutPost(sandboxName)
}

func testLoginPost() (string, error) {
	fmt.Println(">>> testLoginPost")

	rr, err := sendRequest(http.MethodPost, "/login?provider=gitlab", nil, nil, nil, nil, http.StatusCreated, Login)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}
	fmt.Println("Request done")

	var respBody Sandbox
	err = json.Unmarshal([]byte(rr), &respBody)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	time.Sleep(5 * time.Second)

	return respBody.Name, nil
}

func testLogoutPost(sandboxName string) error {
	fmt.Println(">>> testLogoutPost: ", sandboxName)

	_, err := sendRequest(http.MethodPost, "/logout?sandbox_name="+sandboxName, nil, nil, nil, nil, http.StatusOK, Logout)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	fmt.Println("Request done")

	time.Sleep(5 * time.Second)

	return nil
}

func initializeVars() {
	redisAddr = redisTestAddr
	influxAddr = influxTestAddr
	postgisHost = postgresTestHost
	postgisPort = postgresTestPort
	//sandboxName = testScenarioName
	os.Setenv("MEEP_SANDBOX_NAME", testScenarioName)
	os.Setenv("MEEP_PUBLIC_URL", "http://localhost")
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
