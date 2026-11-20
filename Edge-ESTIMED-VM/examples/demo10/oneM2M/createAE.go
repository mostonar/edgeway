package onem2m

import (
	"demo10/utils"
	"fmt"
	"io"
	"log"
	"net/http"
)

// CreateAE creates an Application Entity (AE) on the oneM2M platform
//
// This function creates an AE resource on the oneM2M CSE. The AE represents
// the application in the oneM2M system and provides the entry point for
// application resources and data.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param cse_id The structured ID of the CSE
// @param App_Name The resource name for the application
// @param App_Id The application ID (API identifier)
// @param AE_ID The Application Entity ID for authentication
// @param remote_cse_id The remote CSE ID for access control
// @return error Returns nil on success, or an error if AE creation fails
func CreateAE(platformURL, cse_name, cse_id, App_Name, App_Id, AE_ID, remote_cse_id string) error {
	log.Println("Creating Application Entity (AE)...")

	reqBody := fmt.Sprintf(`{
		"m2m:ae": {
			"rn": "%s",
			"api": "%s",
			"rr": true,
			"lbl": ["Type/SmartCar", "Category/Vehicle"],
			"srv": ["5"],
			"at": ["%s"]
		}
	}`, App_Name, App_Id, remote_cse_id)

	url := fmt.Sprintf("%s/~%s/%s", platformURL, cse_id, cse_name)
	log.Println("url (Create AE):", url)
	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 2, AE_ID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusCreated {
		log.Println("✓ Application Entity (AE) created successfully")
		return nil
	} else if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusForbidden {
		log.Println("⚠ AE already exists")
		return nil
	}

	return fmt.Errorf("failed to create AE: status %d, body: %s", resp.StatusCode, string(body))
}
