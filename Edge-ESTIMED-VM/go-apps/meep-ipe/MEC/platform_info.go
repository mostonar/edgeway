package mec

// PlatformInfo contains platform-related information for a MEC application instance
//
// This structure stores the association between an application instance and its
// corresponding AMS service registration and subscription identifiers, along with
// the node name where the instance is deployed.
//
// @struct PlatformInfo
type PlatformInfo struct {
	AmsServiceID      string `json:"ams_service_id,omitempty"`      // AMS service registration identifier
	AppInstanceID     string `json:"app_instance_id,omitempty"`     // MEC Application Instance ID
	AmsSubscriptionID string `json:"ams_subscription_id,omitempty"` // AMS subscription identifier
	NodeName          string `json:"node_name,omitempty"`           // Name of the MEC host node
}
