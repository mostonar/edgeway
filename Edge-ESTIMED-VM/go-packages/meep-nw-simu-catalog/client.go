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
 * @file client.go
 * @brief HTTP client for NEM GET /v1/scenarios and instance lifecycle.
 *
 * @details
 * Expected JSON:
 * @code{.json}
 * {
 *   "scenarios": [
 *     {"id": "monaco-harbor", "description": "...", "bundle_dir": "/path"},
 *     {"id": "monaco-harbor-lite", "description": "...", "bundle_dir": "/path"}
 *   ]
 * }
 * @endcode
 *
 * A missing `scenarios` field is treated as an empty list, not an error.
 * Transport, HTTP, and JSON failures wrap @ref ErrNemUnavailable.
 *
 * @ingroup nw_simu_catalog
 */
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

/**
 * @defgroup nw_simu_catalog_errors Sentinel errors
 * @ingroup nw_simu_catalog
 * @brief Errors returned by the NEM HTTP client. Listing callers map them to HTTP 502.
 * @{
 */

/**
 * @brief MEEP_NEM_URL is empty after @ref FromEnv.
 *
 * @details Distinct from @ref ErrNemUnavailable so logs can show a config
 * mistake rather than a down NEM.
 */
var ErrNemURLEmpty = errors.New("MEEP_NEM_URL is not set")

/**
 * @brief NEM catalog or instance call could not be completed.
 *
 * @details Wrapped around request, transport, non-2xx HTTP, and JSON errors
 * (`fmt.Errorf("%w: ...", ErrNemUnavailable, ...)`). Use errors.Is to test.
 */
var ErrNemUnavailable = errors.New("network emulator catalog unavailable")

/**
 * @brief NEM POST /v1/instances returned HTTP 409 (sandbox_id already active).
 */
var ErrNemConflict = errors.New("network emulator instance already active")

/**
 * @brief NEM rejected the instance request (HTTP 400, missing ids, invalid JSON).
 */
var ErrNemBadRequest = errors.New("network emulator instance request rejected")

/** User-facing message when NEM cannot be reached (HTTP 502 body / UI). */
const ErrMsgNemNotRunning = "The Network Emulation stack is not running"

/** @} */

/**
 * @brief One scenario entry from NEM GET /v1/scenarios.
 */
type NemScenario struct {
	ID          string `json:"id"`          /**< Catalog id; used as dropdown name. */
	Description string `json:"description"` /**< Human-readable description. */
	BundleDir   string `json:"bundle_dir"`  /**< Path of the scenario bundle on host S. */
}

/**
 * @brief Wire envelope for GET /v1/scenarios.
 * @internal
 */
type nemScenarioList struct {
	Scenarios []NemScenario `json:"scenarios"` /**< Catalog entries; nil is normalized to empty. */
}

/**
 * @brief HTTP client for the NEM scenario catalog and instance lifecycle.
 *
 * @details Created by @ref New. Holds a snapshot of @ref Config, the
 * listing http.Client (GET {NemURL}/v1/scenarios), and a longer-timeout
 * client for POST/DELETE /v1/instances (NEM wait_ready is ~120s).
 */
type Client struct {
	cfg          Config       /**< Snapshot from @ref New. */
	http         *http.Client /**< Listing client; never nil after @ref New. */
	instanceHTTP *http.Client /**< Instance lifecycle client; never nil after @ref New. */
}

/**
 * @brief Constructs a catalog client.
 *
 * @param cfg        Catalog configuration (type, NEM URL, timeouts).
 * @param httpClient Optional HTTP client used for both listing and
 *                   instance calls (tests). If nil, listing uses
 *                   @ref Config::Timeout and instance calls use
 *                   @ref Config::ActivateTimeout.
 * @return Non-nil *Client.
 */
func New(cfg Config, httpClient *http.Client) *Client {
	listing := httpClient
	instance := httpClient
	if listing == nil {
		listing = &http.Client{Timeout: cfg.Timeout()}
	}
	if instance == nil {
		instance = &http.Client{Timeout: cfg.ActivateTimeout()}
	}
	return &Client{cfg: cfg, http: listing, instanceHTTP: instance}
}

/**
 * @brief Returns the configuration snapshot stored in the client.
 *
 * @return Copy of the Config passed to @ref New.
 */
func (c *Client) Cfg() Config {
	return c.cfg
}

/**
 * @brief Fetches the NEM catalog. Does not fall back to CouchDB.
 *
 * @param ctx Optional request context. If non-nil, attached to the GET.
 *            A nil ctx still performs the request without a derived context.
 *
 * @return Slice of @ref NemScenario (possibly empty) and a sentinel error:
 *         - @ref ErrNemUnavailable if c is nil, the request fails, HTTP is
 *           not 2xx, or the body is not valid JSON
 *         - @ref ErrNemURLEmpty if Config.NemURL is empty
 *
 * @note Callers in meep-platform-ctrl map any error to HTTP 502.
 */
func (c *Client) ListNemScenarios(ctx context.Context) ([]NemScenario, error) {
	if c == nil {
		return nil, ErrNemUnavailable
	}
	if c.cfg.NemURL == "" {
		return nil, ErrNemURLEmpty
	}

	url := c.cfg.NemURL + "/v1/scenarios"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNemUnavailable, err)
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNemUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNemUnavailable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrNemUnavailable, resp.StatusCode)
	}

	var list nemScenarioList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", ErrNemUnavailable, err)
	}
	if list.Scenarios == nil {
		list.Scenarios = []NemScenario{}
	}
	return list.Scenarios, nil
}

/**
 * @brief Builds a set of non-empty NEM scenario ids.
 *
 * @details Input is the catalog from @ref Client::ListNemScenarios.
 * Empty ids are skipped. Used as the nemIDs argument of @ref FilterCompanionDocs.
 */
func IDSet(scenarios []NemScenario) map[string]struct{} {
	out := make(map[string]struct{}, len(scenarios))
	for _, s := range scenarios {
		if s.ID != "" {
			out[s.ID] = struct{}{}
		}
	}
	return out
}
