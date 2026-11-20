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
 * @file doc.go
 * @brief Network-emulator catalog and instance client (NEM GET /v1/scenarios, POST/DELETE /v1/instances).
 *
 * @details
 * Package catalog (module
 * `github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-nw-simu-catalog`)
 * is used by **meep-platform-ctrl** (listing) and **meep-sandbox-ctrl**
 * (activate / terminate).
 *
 * When @ref Config::Type is @ref TypeSimu5G or @ref TypeOAI:
 * - GET /scenarios is driven by `GET {nem-url}/v1/scenarios`
 * - scenario selection calls `POST {nem-url}/v1/instances`
 * - de-selection calls `DELETE {nem-url}/v1/instances/{sandbox_id}`
 *
 * This package:
 * - reads the process environment (@ref FromEnv)
 * - calls `GET {nem-url}/v1/scenarios` (@ref Client::ListNemScenarios)
 * - merges NEM ids with CouchDB companion documents, or emits name/id stubs
 *   (@ref MergeNemWithCompanions)
 * - creates and deletes NEM instances (@ref Client::CreateInstance,
 *   @ref Client::DeleteInstance)
 *
 * When type is @ref TypeInternal, platform-ctrl and sandbox-ctrl keep the
 * CouchDB / virt-engine path and do not use this package's NEM path.
 *
 * **Error policy:** NEM down, empty URL, HTTP failure, or invalid JSON maps
 * to @ref ErrNemUnavailable or @ref ErrNemURLEmpty. Callers return HTTP 502.
 * HTTP 409 maps to @ref ErrNemConflict. There is no CouchDB fallback on
 * the NEM listing path.
 *
 * **Frontend contract:** the browser still calls
 * `GET /platform-ctrl/v1/scenarios` and
 * `POST /{sbx}/sandbox-ctrl/v1/active/{name}`. Dropdown `name` is the NEM
 * scenario id (`monaco-harbor`, `monaco-harbor-lite`, …). The browser never
 * calls NEM.
 *
 * @see Config
 * @see Client
 * @see MergeNemWithCompanions
 * @see FilterCompanionDocs
 * @see CreateInstanceRequest
 * @see Instance
 *
 * @defgroup nw_simu_catalog meep-nw-simu-catalog
 * @brief NEM catalog and instance client for MEC Sandbox (Round 5.3).
 */
package catalog
