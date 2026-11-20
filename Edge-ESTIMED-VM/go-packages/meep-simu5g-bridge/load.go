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
 * @file load.go
 * @brief Loads shared integration JSON configuration from disk.
 * @ingroup simu5g_bridge_pkg
 */
package bridge

import (
	"encoding/json"
	"io/ioutil"
)

/**
 * @brief Reads and unmarshals the integration config file used on host B.
 * @param path Filesystem path to JSON (e.g. example-integration-config.json).
 * @return Parsed IntegrationConfig or I/O/JSON error.
 */
func LoadIntegrationConfig(path string) (*IntegrationConfig, error) {
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &IntegrationConfig{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
