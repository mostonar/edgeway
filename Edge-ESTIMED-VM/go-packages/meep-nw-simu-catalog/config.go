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
 * @file config.go
 * @brief Environment-driven catalog switch (simulator type, NEM URL, timeout).
 *
 * @details
 * Values are injected by meepctl from `.meepctl-repocfg.yaml` section
 * `repo.nw_simulator` into Helm `image.env`:
 * - MEEP_NW_SIMULATOR_TYPE          ← repo.nw_simulator.type
 * - MEEP_NEM_URL                    ← repo.nw_simulator.nem-url
 * - MEEP_NEM_TIMEOUT_MS             ← repo.nw_simulator.timeout-ms
 * - MEEP_NEM_ACTIVATE_TIMEOUT_MS    ← repo.nw_simulator.activate-timeout-ms
 *
 * @ingroup nw_simu_catalog
 */
package catalog

import (
	"os"
	"strconv"
	"strings"
	"time"
)

/**
 * @defgroup nw_simu_catalog_types Simulator type constants
 * @ingroup nw_simu_catalog
 * @brief Values of repo.nw_simulator.type / MEEP_NW_SIMULATOR_TYPE.
 * @{
 */

/** Linux TC / CouchDB listing (default). NEM is not used. */
const TypeInternal = "internal"

/** OMNeT++/Simu5G NEM on radio host S. Listing uses GET /v1/scenarios. */
const TypeSimu5G = "simu5g"

/** OAI NEM on radio host S. Listing uses the same NEM catalog contract. */
const TypeOAI = "oai"

/** @} */

/**
 * @defgroup nw_simu_catalog_env Environment variable names
 * @ingroup nw_simu_catalog
 * @brief Process environment keys read by @ref FromEnv.
 * @internal
 * @{
 */

/** MEEP_NW_SIMULATOR_TYPE: internal | simu5g | oai. */
const envType = "MEEP_NW_SIMULATOR_TYPE"

/** MEEP_NEM_URL: base URL of NEM REST, no trailing slash required. */
const envNemURL = "MEEP_NEM_URL"

/** MEEP_NEM_TIMEOUT_MS: listing HTTP client timeout in milliseconds. */
const envTimeoutMs = "MEEP_NEM_TIMEOUT_MS"

/** MEEP_NEM_ACTIVATE_TIMEOUT_MS: POST/DELETE /v1/instances timeout in milliseconds. */
const envActivateTimeoutMs = "MEEP_NEM_ACTIVATE_TIMEOUT_MS"

/** Default type when MEEP_NW_SIMULATOR_TYPE is unset or empty. */
const defaultType = TypeInternal

/** Default NEM listing HTTP timeout (milliseconds) when unset or invalid. */
const defaultTOMs = 3000

/** Default NEM instance activate/delete timeout (milliseconds). NEM wait_ready is 120s. */
const defaultActivateTOMs = 180000

/** @} */

/**
 * @brief Catalog switch loaded from the process environment.
 *
 * @details
 * Type selects the listing backend. NemURL is the NEM REST base
 * (for example `http://172.29.10.20:8787`). TimeoutMs is the listing HTTP
 * timeout used by @ref New when no custom http.Client is supplied.
 * ActivateTimeoutMs is used for POST/DELETE /v1/instances (NEM waits for READY).
 */
type Config struct {
	Type              string /**< @ref TypeInternal, @ref TypeSimu5G, or @ref TypeOAI. */
	NemURL            string /**< NEM base URL with trailing slash stripped. */
	TimeoutMs         int    /**< Listing HTTP timeout in milliseconds; default 3000. */
	ActivateTimeoutMs int    /**< Instance POST/DELETE timeout in milliseconds; default 180000. */
}

/**
 * @brief Loads @ref Config from the MEEP_NEM_* process environment.
 *
 * @details
 * Type is trimmed and lower-cased; empty type becomes @ref TypeInternal.
 * NemURL is trimmed and any trailing `/` is removed.
 * TimeoutMs stays @ref defaultTOMs unless the env value is a positive integer.
 * ActivateTimeoutMs stays @ref defaultActivateTOMs unless the env value is a
 * positive integer.
 *
 * @return Config populated from the environment.
 */
func FromEnv() Config {
	cfg := Config{
		Type:              strings.ToLower(strings.TrimSpace(os.Getenv(envType))),
		NemURL:            strings.TrimRight(strings.TrimSpace(os.Getenv(envNemURL)), "/"),
		TimeoutMs:         defaultTOMs,
		ActivateTimeoutMs: defaultActivateTOMs,
	}
	if cfg.Type == "" {
		cfg.Type = defaultType
	}
	if v := strings.TrimSpace(os.Getenv(envTimeoutMs)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.TimeoutMs = n
		}
	}
	if v := strings.TrimSpace(os.Getenv(envActivateTimeoutMs)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ActivateTimeoutMs = n
		}
	}
	return cfg
}

/**
 * @brief Reports whether listing must be fetched from NEM.
 *
 * @return true when Type is @ref TypeSimu5G or @ref TypeOAI; false for internal.
 */
func (c Config) UsesNem() bool {
	return c.Type == TypeSimu5G || c.Type == TypeOAI
}

/**
 * @brief HTTP client timeout derived from TimeoutMs.
 *
 * @return time.Duration; uses @ref defaultTOMs when TimeoutMs is not positive.
 */
func (c Config) Timeout() time.Duration {
	ms := c.TimeoutMs
	if ms <= 0 {
		ms = defaultTOMs
	}
	return time.Duration(ms) * time.Millisecond
}

/**
 * @brief HTTP client timeout for POST/DELETE /v1/instances.
 *
 * @return time.Duration; uses @ref defaultActivateTOMs when ActivateTimeoutMs is not positive.
 */
func (c Config) ActivateTimeout() time.Duration {
	ms := c.ActivateTimeoutMs
	if ms <= 0 {
		ms = defaultActivateTOMs
	}
	return time.Duration(ms) * time.Millisecond
}
