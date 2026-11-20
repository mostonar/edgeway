package onem2m

import (
	"fmt"
	"io"
	"log"
	"meep-ipe/utils"
)

// CreateContainer creates a Container resource under the Application Entity
//
// This function creates a Container resource in the oneM2M resource tree.
// Containers are used to store ContentInstance resources (data) and provide
// a logical grouping mechanism for related data.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param cse_id The structured ID of the CSE
// @param App_Name The resource name for the application
// @param AE_ID The Application Entity ID for authentication
// @param containerName The name of the container to create
// @param endpointurl The URL of the endpoint
// @return error Returns nil on success, or an error if container creation fails
func CreateContainer(containerName, endpointurl string) error {
	log.Println("Creating Flex Container...")

	reqBody := fmt.Sprintf(`{
		"mec:endpt": {
        	"rn": "%s",
        	"lbl": ["MEC"],
        	"cnd": "org.etsi.mec.moduleclass.endpoint",
        	"uris": ["%s"]
    	}
	}`, containerName, endpointurl)

	url := fmt.Sprintf("%s:%s/~/%s/%s/%s/MecServices", config["CSE-URL"], config["CSE-PORT"],
		config["CSE-ID"], config["CSE-NAME"], NodeName())
	log.Println("url (Create Container):", url)
	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 28, "CAdmin")
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	rsc, err := responseRSC(resp)
	if err != nil {
		return fmt.Errorf("failed to create flex container: %v, body: %s", err, string(body))
	}

	if isCreateSuccess(rsc) {
		if rsc == RscCreated {
			log.Println("✓ Flex Container created successfully")
		} else {
			log.Printf("⚠ Flex Container already exists (RSC %d)", rsc)
		}
		return nil
	}

	return fmt.Errorf("failed to create flex container: RSC %d, body: %s", rsc, string(body))
}
