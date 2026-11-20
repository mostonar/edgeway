package mec

// AppInstanceID represents a MEC Application Instance identifier
//
// This structure contains the identification information for a MEC Application Instance,
// including its unique ID, name, and the node name where it is deployed.
//
// Reference: ETSI GS MEC 011 - MEC Application Lifecycle Management API
//
// @struct AppInstanceID
type AppInstanceID struct {
	ID       string `json:"id"`       // Unique identifier for the application instance
	Name     string `json:"name"`     // Name of the application instance
	NodeName string `json:"nodeName"` // Name of the MEC host node where the instance is deployed
}
