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

package utils

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// objstorePlaceholderMarkers are sentinel values shipped in the MEEP
// objstore config templates (config/objstore-thanos.yaml,
// config/objstore-thanos-archive.yaml). If a deployed objstore secret still
// contains any of these, the user has not completed the manual long-term
// storage setup documented in the README.
var objstorePlaceholderMarkers = []string{
	"metrics.try-mec.etsi.org",
	"my-access-key",
	"my-secret-key",
}

// objstoreFallbackMarker is stamped into any objstore.yml this package
// generates, so a later call can recognize "already running on our own
// fallback" and tell it apart from a real user-supplied config. Without
// this, a fallback secret written by one component's check (e.g.
// meep-prometheus) would look "configured" to the next component's check
// (e.g. meep-thanos) purely because it no longer contains the *original*
// placeholder markers — silently skipping rustfs.enabled for the release
// that actually needs to deploy it.
const objstoreFallbackMarker = "# meepctl-managed-fallback-objstore: do not edit manually, see README for the real long-term storage setup"

// IsObjstoreConfigured reports whether the named objstore secret holds a
// real, user-supplied config — i.e. it exists, isn't empty, doesn't still
// carry the shipped placeholder values, and wasn't itself written by our
// own fallback (see objstoreFallbackMarker). Returns false whenever the
// fallback should be (re-)applied.
func IsObjstoreConfigured(secretName string, cobraCmd *cobra.Command) bool {
	cmd := exec.Command("kubectl", "get", "secret", secretName, "-o", `jsonpath={.data.objstore\.yml}`)
	out, err := ExecuteCmd(cmd, cobraCmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(out))
	if err != nil {
		return false
	}
	content := string(decoded)
	if strings.Contains(content, objstoreFallbackMarker) {
		return false
	}
	for _, marker := range objstorePlaceholderMarkers {
		if strings.Contains(content, marker) {
			return false
		}
	}
	return true
}

// EnsureFallbackObjstore patches the named secret with connection details
// for the RustFS instance bundled in the thanos chart (deployed as
// releaseName, holding bucketName) whenever the user has not completed the
// manual object store setup documented in the README. This lets Thanos
// (and Prometheus' Thanos sidecar, which reads the same secret) come up
// healthy against an in-cluster, dev/test-only object store instead of
// crash-looping against the shipped placeholder endpoint.
//
// Idempotent and order-independent: whether this is the first call to spot
// the missing config, or a later call re-confirming a fallback secret
// written by a different component earlier in the same deploy run, it
// always (re-)writes the fallback config and returns true — so every
// caller that needs to flip on rustfs.enabled for its own release actually
// does so. Leaves the secret untouched, and returns false, only when the
// user has configured a real object store.
func EnsureFallbackObjstore(secretName string, releaseName string, bucketName string, cobraCmd *cobra.Command) bool {
	if IsObjstoreConfigured(secretName, cobraCmd) {
		return false
	}

	fmt.Println("Object store not configured for secret [" + secretName + "]; falling back to bundled RustFS instance (dev/test only)")

	config := fmt.Sprintf(`%s
type: s3
config:
  bucket: %s
  endpoint: %s-rustfs-svc:9000
  access_key: rustfsadmin
  secret_key: rustfsadmin
  insecure: true
`, objstoreFallbackMarker, bucketName, releaseName)

	genCmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
		"--from-literal=objstore.yml="+config,
		"--dry-run=client", "-o", "yaml")
	manifest, err := ExecuteCmd(genCmd, cobraCmd)
	if err != nil {
		fmt.Println("Error generating fallback objstore secret for [" + secretName + "]")
		fmt.Println(manifest)
		return false
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(manifest)
	if _, err := ExecuteCmd(applyCmd, cobraCmd); err != nil {
		fmt.Println("Error applying fallback objstore secret for [" + secretName + "]")
		return false
	}

	return true
}
