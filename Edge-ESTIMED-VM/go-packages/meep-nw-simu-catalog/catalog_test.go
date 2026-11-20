/*
 * Copyright (c) 2026  The AdvantEDGE Authors
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

/**
 * @file catalog_test.go
 * @brief Unit tests for meep-nw-simu-catalog (config, NEM client, merge/filter).
 * @ingroup nw_simu_catalog
 */
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

/** @brief FromEnv defaults to internal type, empty URL, and 3000 ms timeout. */
func TestFromEnvDefaults(t *testing.T) {
	os.Unsetenv("MEEP_NW_SIMULATOR_TYPE")
	os.Unsetenv("MEEP_NEM_URL")
	os.Unsetenv("MEEP_NEM_TIMEOUT_MS")
	os.Unsetenv("MEEP_NEM_ACTIVATE_TIMEOUT_MS")
	cfg := FromEnv()
	if cfg.Type != TypeInternal {
		t.Fatalf("type: got %q want %q", cfg.Type, TypeInternal)
	}
	if cfg.UsesNem() {
		t.Fatal("internal must not use NEM")
	}
	if cfg.Timeout() != 3000*time.Millisecond {
		t.Fatalf("timeout: got %v", cfg.Timeout())
	}
	if cfg.ActivateTimeout() != 180000*time.Millisecond {
		t.Fatalf("activate timeout: got %v", cfg.ActivateTimeout())
	}
}

/** @brief FromEnv reads simu5g, strips trailing slash on NEM URL, honors timeout. */
func TestFromEnvSimu5G(t *testing.T) {
	os.Setenv("MEEP_NW_SIMULATOR_TYPE", "simu5g")
	os.Setenv("MEEP_NEM_URL", "http://172.29.10.20:8787/")
	os.Setenv("MEEP_NEM_TIMEOUT_MS", "1500")
	os.Setenv("MEEP_NEM_ACTIVATE_TIMEOUT_MS", "210000")
	defer func() {
		os.Unsetenv("MEEP_NW_SIMULATOR_TYPE")
		os.Unsetenv("MEEP_NEM_URL")
		os.Unsetenv("MEEP_NEM_TIMEOUT_MS")
		os.Unsetenv("MEEP_NEM_ACTIVATE_TIMEOUT_MS")
	}()
	cfg := FromEnv()
	if cfg.Type != TypeSimu5G || !cfg.UsesNem() {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.NemURL != "http://172.29.10.20:8787" {
		t.Fatalf("nem url: %q", cfg.NemURL)
	}
	if cfg.TimeoutMs != 1500 {
		t.Fatalf("timeout ms: %d", cfg.TimeoutMs)
	}
	if cfg.ActivateTimeoutMs != 210000 {
		t.Fatalf("activate timeout ms: %d", cfg.ActivateTimeoutMs)
	}
}

/** @brief ListNemScenarios parses GET /v1/scenarios JSON from a test HTTP server. */
func TestListNemScenarios(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/scenarios" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"scenarios": []map[string]string{
				{"id": "monaco-harbor", "description": "full", "bundle_dir": "/s/monaco-harbor"},
				{"id": "monaco-harbor-lite", "description": "lite", "bundle_dir": "/s/lite"},
			},
		})
	}))
	defer ts.Close()

	c := New(Config{Type: TypeSimu5G, NemURL: ts.URL, TimeoutMs: 1000}, ts.Client())
	list, err := c.ListNemScenarios(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "monaco-harbor" {
		t.Fatalf("list: %+v", list)
	}
}

/** @brief ListNemScenarios wraps a non-2xx NEM response as ErrNemUnavailable. */
func TestListNemScenariosDown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer ts.Close()

	c := New(Config{Type: TypeSimu5G, NemURL: ts.URL, TimeoutMs: 1000}, ts.Client())
	_, err := c.ListNemScenarios(context.Background())
	if err == nil || !errors.Is(err, ErrNemUnavailable) {
		t.Fatalf("want ErrNemUnavailable, got %v", err)
	}
}

/** @brief ListNemScenarios returns ErrNemURLEmpty when NemURL is unset. */
func TestListNemScenariosEmptyURL(t *testing.T) {
	c := New(Config{Type: TypeSimu5G, NemURL: "", TimeoutMs: 1000}, nil)
	_, err := c.ListNemScenarios(context.Background())
	if !errors.Is(err, ErrNemURLEmpty) {
		t.Fatalf("want ErrNemURLEmpty, got %v", err)
	}
}

/** @brief FilterCompanionDocs keeps CouchDB docs matching NEM ids; skips invalid JSON. */
func TestFilterCompanionDocs(t *testing.T) {
	docs := [][]byte{
		[]byte(`{"name":"monaco-harbor","description":"companion"}`),
		[]byte(`{"name":"4g-macro","description":"internal only"}`),
		[]byte(`{"id":"monaco-harbor-lite","name":"monaco-harbor-lite"}`),
		[]byte(`not-json`),
	}
	nem := IDSet([]NemScenario{
		{ID: "monaco-harbor"},
		{ID: "monaco-harbor-lite"},
		{ID: "unmapped"},
	})
	got := FilterCompanionDocs(docs, nem)
	if len(got) != 2 {
		t.Fatalf("got %d docs", len(got))
	}
	if n := len(FilterCompanionDocs(docs, map[string]struct{}{})); n != 0 {
		t.Fatalf("empty nem ids should yield empty list, got %d", n)
	}
}

/** @brief MergeNemWithCompanions prefers a CouchDB companion and stubs unmatched NEM ids. */
func TestMergeNemWithCompanions(t *testing.T) {
	docs := [][]byte{
		[]byte(`{"name":"monaco-harbor","description":"companion yaml"}`),
		[]byte(`{"name":"4g-macro"}`),
	}
	nem := []NemScenario{
		{ID: "monaco-harbor", Description: "from nem"},
		{ID: "monaco-harbor-lite", Description: "lite"},
	}
	got := MergeNemWithCompanions(nem, docs)
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
	var first scenarioRef
	_ = json.Unmarshal(got[0], &first)
	if first.Name != "monaco-harbor" {
		t.Fatalf("first name %q", first.Name)
	}
	var stub struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(got[1], &stub)
	if stub.Name != "monaco-harbor-lite" || stub.Description != "lite" {
		t.Fatalf("stub %+v", stub)
	}
}

func readyPayload(sandboxID, scenarioID string) map[string]interface{} {
	return map[string]interface{}{
		"sandbox_id":  sandboxID,
		"state":       "READY",
		"scenario_id": scenarioID,
		"slot":        0,
		"bridge_port": 5555,
		"bridge": map[string]interface{}{
			"host":   "lab-oai.etsi.org",
			"port":   5555,
			"target": "lab-oai.etsi.org:5555",
		},
		"endpoints": map[string]interface{}{
			"sim_host":          "lab-oai.etsi.org",
			"traci_port":        9999,
			"bridge_port":       5555,
			"rni_udp_port":      9201,
			"ue_telemetry_port": 9100,
		},
	}
}

/** @brief CreateInstance parses the 201 READY payload and stores sandbox_id as instance-id. */
func TestCreateInstance(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/instances" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body CreateInstanceRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if body.SandboxID != "sbxe0xlax8" || body.ScenarioID != "monaco-harbor" || body.SimTimeLimit != DefaultSimTimeLimit {
			t.Errorf("body %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(readyPayload(body.SandboxID, body.ScenarioID))
	}))
	defer ts.Close()

	c := New(Config{Type: TypeSimu5G, NemURL: ts.URL, TimeoutMs: 1000, ActivateTimeoutMs: 5000}, ts.Client())
	inst, err := c.CreateInstance(context.Background(), CreateInstanceRequest{
		SandboxID:  "sbxe0xlax8",
		ScenarioID: "monaco-harbor",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.SandboxID != "sbxe0xlax8" || inst.State != "READY" || inst.Bridge.Target != "lab-oai.etsi.org:5555" {
		t.Fatalf("instance %+v", inst)
	}
	if inst.Endpoints.RniUDPPort != 9201 || inst.BridgePort != 5555 {
		t.Fatalf("endpoints %+v", inst.Endpoints)
	}
}

/** @brief CreateInstance maps HTTP 409 to ErrNemConflict. */
func TestCreateInstanceConflict(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "sbxe0xlax8 already active; deactivate first"})
	}))
	defer ts.Close()

	c := New(Config{Type: TypeSimu5G, NemURL: ts.URL, TimeoutMs: 1000}, ts.Client())
	_, err := c.CreateInstance(context.Background(), CreateInstanceRequest{
		SandboxID:  "sbxe0xlax8",
		ScenarioID: "monaco-harbor",
	})
	if err == nil || !errors.Is(err, ErrNemConflict) {
		t.Fatalf("want ErrNemConflict, got %v", err)
	}
}

/** @brief CreateInstance requires sandbox_id and scenario_id. */
func TestCreateInstanceMissingIDs(t *testing.T) {
	c := New(Config{Type: TypeSimu5G, NemURL: "http://127.0.0.1:1", TimeoutMs: 1000}, nil)
	_, err := c.CreateInstance(context.Background(), CreateInstanceRequest{})
	if err == nil || !errors.Is(err, ErrNemBadRequest) {
		t.Fatalf("want ErrNemBadRequest, got %v", err)
	}
}

/** @brief DeleteInstance treats 204 and 404 as success. */
func TestDeleteInstance(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		if r.Method != http.MethodDelete {
			t.Errorf("method %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := New(Config{Type: TypeSimu5G, NemURL: ts.URL, TimeoutMs: 1000}, ts.Client())
	if err := c.DeleteInstance(context.Background(), "sbxe0xlax8", false); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/instances/sbxe0xlax8" {
		t.Fatalf("path %s", gotPath)
	}
}

/** @brief DeleteInstance appends retain_artifacts and accepts 404. */
func TestDeleteInstanceNotFoundAndRetain(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		http.NotFound(w, r)
	}))
	defer ts.Close()

	c := New(Config{Type: TypeSimu5G, NemURL: ts.URL, TimeoutMs: 1000}, ts.Client())
	if err := c.DeleteInstance(context.Background(), "sbxe0xlax8", true); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/instances/sbxe0xlax8?retain_artifacts=true" {
		t.Fatalf("path %s", gotPath)
	}
}

/** @brief CreateInstance maps HTTP 500 JSON error to ErrNemUnavailable. */
func TestCreateInstanceHTTP500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "sudo password required for /path/mhmu; set SUDO_PASSWORD_ENV",
		})
	}))
	defer ts.Close()

	c := New(Config{Type: TypeSimu5G, NemURL: ts.URL, TimeoutMs: 1000}, ts.Client())
	_, err := c.CreateInstance(context.Background(), CreateInstanceRequest{
		SandboxID:  "sbx4lesbpo",
		ScenarioID: "monaco-harbor",
	})
	if err == nil || !errors.Is(err, ErrNemUnavailable) {
		t.Fatalf("want ErrNemUnavailable, got %v", err)
	}
	msg := UserMessage(err)
	if msg != "sudo password required for /path/mhmu; set SUDO_PASSWORD_ENV" {
		t.Fatalf("user message: %q", msg)
	}
	if Unreachable(err) {
		t.Fatal("HTTP 500 from NEM is reachable")
	}
}

/** @brief Unreachable is true for transport failures, false when NEM returned HTTP. */
func TestUnreachable(t *testing.T) {
	if !Unreachable(ErrNemURLEmpty) {
		t.Fatal("empty URL should be unreachable")
	}
	if Unreachable(fmt.Errorf("%w: HTTP 500: boom", ErrNemUnavailable)) {
		t.Fatal("HTTP 500 should be reachable")
	}
	if !Unreachable(fmt.Errorf("%w: dial timeout", ErrNemUnavailable)) {
		t.Fatal("dial timeout should be unreachable")
	}
}
