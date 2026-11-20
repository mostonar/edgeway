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
 * @file filter.go
 * @brief Merge and filter helpers for NEM ids versus CouchDB scenario documents.
 *
 * @details
 * Listing for simu5g/oai uses @ref MergeNemWithCompanions so the UI dropdown
 * always contains every NEM id. A CouchDB companion (imported YAML whose
 * `name` or `id` equals the NEM id) is preferred; otherwise a stub
 * `{"id","name","description"}` is emitted. Stubs keep the frontend from
 * treating an empty `scenarios` omitempty payload as “do not update the list”.
 *
 * @ref FilterCompanionDocs is the stricter helper: only CouchDB documents
 * that match a NEM id are kept. NEM ids with no companion are dropped
 * from the filtered list. Platform-ctrl listing uses merge, not filter.
 * Activate of a stub name still POSTs NEM and skips virt-engine until a
 * CouchDB companion exists.
 *
 * @ingroup nw_simu_catalog
 */
package catalog

import "encoding/json"

/**
 * @brief Minimal CouchDB scenario identity fields used for matching.
 * @internal
 */
type scenarioRef struct {
	Name string `json:"name"` /**< Scenario name; compared to NEM id. */
	ID   string `json:"id"`   /**< Optional document id; also compared to NEM id. */
}

/**
 * @brief Keeps CouchDB documents whose name or id is in nemIDs.
 *
 * @param docs   Raw CouchDB scenario JSON documents.
 * @param nemIDs Set of NEM ids from @ref IDSet.
 *
 * @return Matching documents (possibly empty). Invalid JSON is skipped.
 *         Empty nemIDs or empty docs yield an empty (non-nil) slice.
 *
 * @note NEM ids with no companion are omitted. Prefer
 *       @ref MergeNemWithCompanions for the platform-ctrl dropdown.
 */
func FilterCompanionDocs(docs [][]byte, nemIDs map[string]struct{}) [][]byte {
	if len(nemIDs) == 0 || len(docs) == 0 {
		return [][]byte{}
	}
	out := make([][]byte, 0, len(docs))
	for _, doc := range docs {
		var ref scenarioRef
		if err := json.Unmarshal(doc, &ref); err != nil {
			continue
		}
		if _, ok := nemIDs[ref.Name]; ok {
			out = append(out, doc)
			continue
		}
		if ref.ID != "" {
			if _, ok := nemIDs[ref.ID]; ok {
				out = append(out, doc)
			}
		}
	}
	return out
}

/**
 * @brief Indexes CouchDB documents by name and by id.
 *
 * @param docs Raw CouchDB scenario JSON documents.
 * @return Map from name and/or id to the original document bytes.
 *         Invalid JSON is skipped. If both name and id are set, both keys
 *         point at the same document.
 * @internal
 */
func companionIndex(docs [][]byte) map[string][]byte {
	idx := make(map[string][]byte, len(docs))
	for _, doc := range docs {
		var ref scenarioRef
		if err := json.Unmarshal(doc, &ref); err != nil {
			continue
		}
		if ref.Name != "" {
			idx[ref.Name] = doc
		}
		if ref.ID != "" {
			idx[ref.ID] = doc
		}
	}
	return idx
}

/**
 * @brief Builds the GET /scenarios payload when type is simu5g or oai.
 *
 * @details
 * For each NEM entry with a non-empty id:
 * - if a CouchDB document is indexed under that id, that document is used;
 * - otherwise a stub `{"id":id,"name":id,"description":...}` is marshalled
 *   so the frontend listbox receives a `name` field.
 *
 * Order follows the NEM catalog. CouchDB-only names (for example `4g-macro`)
 * are not included.
 *
 * @param nem  NEM catalog from @ref Client::ListNemScenarios.
 * @param docs Raw CouchDB scenario documents (companions and internal YAMLs).
 *
 * @return JSON documents suitable for meep-model JSONMarshallScenarioList.
 */
func MergeNemWithCompanions(nem []NemScenario, docs [][]byte) [][]byte {
	idx := companionIndex(docs)
	out := make([][]byte, 0, len(nem))
	for _, s := range nem {
		if s.ID == "" {
			continue
		}
		if doc, ok := idx[s.ID]; ok {
			out = append(out, doc)
			continue
		}
		stub, err := json.Marshal(struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		}{ID: s.ID, Name: s.ID, Description: s.Description})
		if err != nil {
			continue
		}
		out = append(out, stub)
	}
	return out
}
