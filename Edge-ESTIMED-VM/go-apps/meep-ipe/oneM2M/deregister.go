package onem2m

import (
	"fmt"
	"io"
	"log"
	"meep-ipe/utils"
	"net/http"
)

// DeregisterAE deregisters the Application Entity from the oneM2M platform
//
// This function removes the AE resource from the oneM2M CSE, effectively
// unregistering the application from the platform. This should be called
// during application shutdown or when migrating to a different platform.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cseName The name of the Common Services Entity (CSE)
// @param cse_id The structured ID of the CSE
// @param appName The resource name for the application
// @param aeID The Application Entity ID for authentication
// @return error Returns nil on success, or an error if deregistration fails
func DeregisterAE() error {
	url := fmt.Sprintf("%s:%s/~/%s/%s/%s", config["CSE-URL"], config["CSE-PORT"],
		config["CSE-ID"], config["CSE-NAME"], config["mecAppName"])

	log.Printf("Deregistering AE with URL: %s", url)

	resp, err := utils.MakeOneM2MRequest("DELETE", url, "", 2, config["AE-ID"].(string))
	if err != nil {
		return fmt.Errorf("failed to deregister AE: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to deregister AE, status: %s, response: %s", resp.Status, string(body))
	}

	log.Printf("✓ AE deregistered successfully, response: %s", string(body))
	return nil
}
