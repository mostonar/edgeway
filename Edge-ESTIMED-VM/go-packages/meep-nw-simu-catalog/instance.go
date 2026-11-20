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
 * @file instance.go
 * @brief HTTP client for NEM instance lifecycle (POST/GET/DELETE /v1/instances).
 *
 * @details
 * NEM POST /v1/instances is synchronous until READY (wait_ready ~120s).
 * The sandbox_id is the MEC sandbox name (for example `sbxe0xlax8`);
 * scenario_id is the NEM catalog id (`monaco-harbor`).
 *
 * @see monaco-harbor-multiusers/docs/NEM.md
 * @ingroup nw_simu_catalog
 */
package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

/** Default sim_time_limit sent when CreateInstanceRequest.SimTimeLimit is empty. */
const DefaultSimTimeLimit = "1800s" // Half an hour

/**
 * @brief POST /v1/instances request body.
 */
type CreateInstanceRequest struct {
	SandboxID    string `json:"sandbox_id"`               /**< MEC sandbox name; NEM instance-id. */
	ScenarioID   string `json:"scenario_id"`              /**< NEM catalog id (monaco-harbor). */
	SimTimeLimit string `json:"sim_time_limit,omitempty"` /**< opp_run limit; default 300s. */
	WebhookURL   string `json:"webhook_url,omitempty"`    /**< Optional Host-B callback. */
}

/**
 * @brief TCP target on host S for Simu5gBridgeInterface.
 */
type InstanceBridge struct {
	Host   string `json:"host"`   /**< DNS or IP of host S as seen from B. */
	Port   int    `json:"port"`   /**< Allocated bridge TCP port. */
	Target string `json:"target"` /**< host:port string for the bridge client. */
}

/**
 * @brief Per-instance ports allocated by NEM.
 */
type InstanceEndpoints struct {
	SimHost         string `json:"sim_host"`          /**< Host S address. */
	TraciPort       int    `json:"traci_port"`        /**< Veins TraCI port. */
	BridgePort      int    `json:"bridge_port"`       /**< Simu5gBridge TCP port. */
	RniUDPPort      int    `json:"rni_udp_port"`      /**< RNIS UDP ingest. */
	UeTelemetryPort int    `json:"ue_telemetry_port"` /**< UE telemetry ingest. */
}

/**
 * @brief READY payload from POST /v1/instances (201) and GET /v1/instances/{id}.
 */
type Instance struct {
	SandboxID  string            `json:"sandbox_id"`  /**< NEM instance-id (MEC sandbox name). */
	State      string            `json:"state"`       /**< READY after a successful POST. */
	ScenarioID string            `json:"scenario_id"` /**< NEM catalog id. */
	Slot       int               `json:"slot"`        /**< NEM slot index. */
	BridgePort int               `json:"bridge_port"` /**< Same as Bridge.Port / Endpoints.BridgePort. */
	Bridge     InstanceBridge    `json:"bridge"`      /**< Host B connect target. */
	Endpoints  InstanceEndpoints `json:"endpoints"`   /**< Full port set for later adapters. */
}

/**
 * @brief Creates a NEM instance and waits until READY.
 *
 * @param ctx Optional request context. If nil, the instance HTTP client timeout applies.
 * @param req sandbox_id and scenario_id are required. Empty SimTimeLimit becomes @ref DefaultSimTimeLimit.
 *
 * @return Parsed READY payload. sandbox_id is the instance-id to store and pass to DeleteInstance.
 *
 * @retval ErrNemURLEmpty     Config.NemURL is empty.
 * @retval ErrNemBadRequest   missing ids, HTTP 400.
 * @retval ErrNemConflict     HTTP 409 (already active).
 * @retval ErrNemUnavailable  nil client, transport, HTTP 5xx, or invalid JSON.
 */
func (c *Client) CreateInstance(ctx context.Context, req CreateInstanceRequest) (*Instance, error) {
	if c == nil {
		return nil, ErrNemUnavailable
	}
	if c.cfg.NemURL == "" {
		return nil, ErrNemURLEmpty
	}
	req.SandboxID = strings.TrimSpace(req.SandboxID)
	req.ScenarioID = strings.TrimSpace(req.ScenarioID)
	if req.SandboxID == "" || req.ScenarioID == "" {
		return nil, fmt.Errorf("%w: sandbox_id and scenario_id are required", ErrNemBadRequest)
	}
	if strings.TrimSpace(req.SimTimeLimit) == "" {
		req.SimTimeLimit = DefaultSimTimeLimit
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNemUnavailable, err)
	}

	raw, status, err := c.doInstance(ctx, http.MethodPost, c.cfg.NemURL+"/v1/instances", body)
	if err != nil {
		return nil, err
	}
	if status == http.StatusConflict {
		return nil, fmt.Errorf("%w: %s", ErrNemConflict, errorMessage(raw, status))
	}
	if status == http.StatusBadRequest {
		return nil, fmt.Errorf("%w: %s", ErrNemBadRequest, errorMessage(raw, status))
	}
	if status != http.StatusCreated && (status < 200 || status >= 300) {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrNemUnavailable, status, errorMessage(raw, status))
	}

	var inst Instance
	if err := json.Unmarshal(raw, &inst); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", ErrNemUnavailable, err)
	}
	if inst.SandboxID == "" {
		inst.SandboxID = req.SandboxID
	}
	if inst.ScenarioID == "" {
		inst.ScenarioID = req.ScenarioID
	}
	if !InstanceReady(&inst) {
		return nil, fmt.Errorf("%w: HTTP %d: instance %s state %s", ErrNemUnavailable, status, inst.SandboxID, inst.State)
	}
	return &inst, nil
}

/**
 * @brief Fetches NEM instance status (GET /v1/instances/{sandbox_id}).
 */
func (c *Client) GetInstance(ctx context.Context, sandboxID string) (*Instance, error) {
	if c == nil {
		return nil, ErrNemUnavailable
	}
	if c.cfg.NemURL == "" {
		return nil, ErrNemURLEmpty
	}
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return nil, fmt.Errorf("%w: sandbox_id is required", ErrNemBadRequest)
	}

	path := c.cfg.NemURL + "/v1/instances/" + url.PathEscape(sandboxID)
	raw, status, err := c.doInstance(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusBadRequest {
		return nil, fmt.Errorf("%w: %s", ErrNemBadRequest, errorMessage(raw, status))
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrNemUnavailable, status, errorMessage(raw, status))
	}

	var inst Instance
	if err := json.Unmarshal(raw, &inst); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", ErrNemUnavailable, err)
	}
	if inst.SandboxID == "" {
		inst.SandboxID = sandboxID
	}
	return &inst, nil
}

/**
 * @brief Deletes the NEM instance (DELETE /v1/instances/{sandbox_id}).
 *
 * @details HTTP 204, 200, and 404 are success (404 = already gone).
 *
 * @param retainArtifacts when true, appends ?retain_artifacts=true.
 */
func (c *Client) DeleteInstance(ctx context.Context, sandboxID string, retainArtifacts bool) error {
	if c == nil {
		return ErrNemUnavailable
	}
	if c.cfg.NemURL == "" {
		return ErrNemURLEmpty
	}
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return fmt.Errorf("%w: sandbox_id is required", ErrNemBadRequest)
	}

	path := c.cfg.NemURL + "/v1/instances/" + url.PathEscape(sandboxID)
	if retainArtifacts {
		path += "?retain_artifacts=false"
	}
	raw, status, err := c.doInstance(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusNoContent || (status >= 200 && status < 300) {
		return nil
	}
	if status == http.StatusBadRequest {
		return fmt.Errorf("%w: %s", ErrNemBadRequest, errorMessage(raw, status))
	}
	return fmt.Errorf("%w: HTTP %d: %s", ErrNemUnavailable, status, errorMessage(raw, status))
}

func (c *Client) doInstance(ctx context.Context, method, urlStr string, body []byte) ([]byte, int, error) {
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, urlStr, rdr)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrNemUnavailable, err)
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.instanceHTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrNemUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: %v", ErrNemUnavailable, err)
	}
	return raw, resp.StatusCode, nil
}

func errorMessage(raw []byte, status int) string {
	var wrap struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Error != "" {
		return wrap.Error
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return s
}

/** READY is the only NEM state that means the emulation stack is running. */
func InstanceReady(inst *Instance) bool {
	if inst == nil {
		return false
	}
	return strings.ToUpper(strings.TrimSpace(inst.State)) == "READY"
}

/**
 * @brief User-facing text for an activate/delete error (HTTP 502 body / UI).
 *
 * @details Prefers the NEM JSON `error` field embedded after `HTTP NNN: `.
 * Falls back to @ref ErrMsgNemNotRunning for empty URL / generic unavailability.
 */
func UserMessage(err error) string {
	if err == nil {
		return ErrMsgNemNotRunning
	}
	s := err.Error()
	if i := strings.Index(s, "HTTP "); i >= 0 {
		rest := s[i:]
		if j := strings.Index(rest, ": "); j >= 0 {
			msg := strings.TrimSpace(rest[j+2:])
			if msg != "" {
				return msg
			}
		}
	}
	if errors.Is(err, ErrNemURLEmpty) || errors.Is(err, ErrNemUnavailable) {
		return ErrMsgNemNotRunning
	}
	return s
}

/**
 * @brief Reports whether err means NEM HTTP was not reached.
 *
 * @details Transport failures and an empty URL are unreachable. An HTTP
 * status from NEM (including 409/400/500) means NEM is accessible.
 */
func Unreachable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNemURLEmpty) {
		return true
	}
	if errors.Is(err, ErrNemConflict) || errors.Is(err, ErrNemBadRequest) {
		return false
	}
	if strings.Contains(err.Error(), "HTTP ") {
		return false
	}
	return errors.Is(err, ErrNemUnavailable)
}
