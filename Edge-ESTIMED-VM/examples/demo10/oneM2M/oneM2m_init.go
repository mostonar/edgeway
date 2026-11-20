package onem2m

import (
	"log"
	"time"
)

// InitOneM2MApp initializes a oneM2M application by creating an AE and container
//
// This function performs the complete initialization sequence for a oneM2M application:
//   1. Creates an Application Entity (AE) on the oneM2M platform
//   2. Waits briefly to ensure the AE is registered
//   3. Creates a Container under the AE for storing content instances
//   4. Verifies that the container was created successfully
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param cse_id The structured ID of the CSE
// @param App_Name The resource name for the application
// @param App_Id The application ID
// @param AE_ID The Application Entity ID
// @param containerName The name of the container to create
// @param remote_cse_id The remote CSE ID for access control
// @return error Returns nil on success, or an error if initialization fails
func InitOneM2MApp(platformURL, cse_name, cse_id, App_Name,
	App_Id, AE_ID, containerName, remote_cse_id string) error {
	log.Printf("Connecting to MN-CSE: %s", platformURL)
	// Create AE
	if err := CreateAE(platformURL, cse_name, cse_id, App_Name, App_Id, AE_ID, remote_cse_id); err != nil {
		return err
	}
	// Wait briefly to ensure AE is registered
	time.Sleep(2 * time.Second)

	// Create Container
	if err := CreateContainer(platformURL, cse_name, cse_id, App_Name, AE_ID, containerName, remote_cse_id); err != nil {
		return err
	}

	// Verify container
	if err := RetrieveContainer(platformURL, cse_name, cse_id, App_Name, AE_ID, containerName); err != nil {
		log.Printf("Warning: Could not verify container: %v", err)
	}

	log.Println("✓ Application initialized successfully")
	return nil
}
