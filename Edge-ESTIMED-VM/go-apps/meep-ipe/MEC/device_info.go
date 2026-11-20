package mec

// This file models the request/response body for the ETSI GS MEC 033 IoT API
// POST /registered_devices endpoint (operationId: registereddevicesPOST),
// as defined in iot.yaml.

// RegisteredDevicesBody is the top-level request body wrapper for
// POST /registered_devices (schema: registered_devices_body).
type RegisteredDevicesBody struct {
	DeviceInfo *DeviceInfo `json:"DeviceInfo"`
}

// DeviceInfo describes an IoT device to be registered (schema: DeviceInfo).
//
// NOTE 1: At least one attribute among Gpsi, Pei, Supi, Msisdn, Imei, Imsi,
// and Iccid should be provided.
// NOTE 2: Until a valid traffic rule is provided, the device will not be
// able to use the IoTS.
type DeviceInfo struct {
	// Required fields
	DeviceAuthenticationInfo string `json:"deviceAuthenticationInfo"`
	DeviceId                 string `json:"deviceId"`
	Enabled                  bool   `json:"enabled"`

	// Optional fields
	DeviceMetadata               []KeyValuePair                `json:"deviceMetadata,omitempty"`
	Gpsi                         string                        `json:"gpsi,omitempty"`
	Pei                          string                        `json:"pei,omitempty"`
	Supi                         string                        `json:"supi,omitempty"`
	Msisdn                       string                        `json:"msisdn,omitempty"`
	Imei                         string                        `json:"imei,omitempty"`
	Imsi                         string                        `json:"imsi,omitempty"`
	Iccid                        string                        `json:"iccid,omitempty"`
	RequestedMecTrafficRule      []TrafficRuleDescriptor        `json:"requestedMecTrafficRule,omitempty"`
	RequestedIotPlatformId       string                        `json:"requestedIotPlatformId,omitempty"`
	RequestedUserTransportId     string                        `json:"requestedUserTransportId,omitempty"`
	DeviceSpecificMessageFormats *DeviceSpecificMessageFormats `json:"deviceSpecificMessageFormats,omitempty"`
	DownlinkInfo                 *DownlinkInfo                 `json:"downlinkInfo,omitempty"`
	ClientCertificate             string                        `json:"clientCertificate,omitempty"`
}

// KeyValuePair holds device metadata as a key-value pair (schema: KeyValuePair).
type KeyValuePair struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// DeviceSpecificMessageFormats describes the format of messages published by
// the MEC IoTS on behalf of the IoT device (schema: DeviceSpecificMessageFormats).
type DeviceSpecificMessageFormats struct {
	EventMsgFormat  *EventMsg  `json:"eventMsgFormat,omitempty"`
	UplinkMsgFormat *UplinkMsg `json:"uplinkMsgFormat,omitempty"`
}

const (
	SerializerTypeJSON     SerializerType = "JSON"
	SerializerTypeXML      SerializerType = "XML"
	SerializerTypeProtobuf SerializerType = "PROTOBUF3"
)

// EventMsg describes the event message format (schema: EventMsg).
type EventMsg struct {
	// Required
	EventTopic         string         `json:"eventTopic"`
	SelectedSerializer SerializerType `json:"selectedSerializer"`

	// Optional
	IncludeDeviceAddr     bool `json:"includeDeviceAddr,omitempty"`
	IncludeDeviceMetadata bool `json:"includeDeviceMetadata,omitempty"`
	IncludePei            bool `json:"includePei,omitempty"`
	IncludeSupi           bool `json:"includeSupi,omitempty"`
	IncludeImei           bool `json:"includeImei,omitempty"`
	IncludeImsi           bool `json:"includeImsi,omitempty"`
	IncludeIccid          bool `json:"includeIccid,omitempty"`
	IncludeDeviceId       bool `json:"includeDeviceId,omitempty"`
}

// UplinkMsg describes the uplink message format (schema: UplinkMsg).
type UplinkMsg struct {
	// Required
	UplinkTopic        string         `json:"uplinkTopic"`
	SelectedSerializer SerializerType `json:"selectedSerializer"`

	// Optional
	IncludeDevicePort     bool `json:"includeDevicePort,omitempty"`
	IncludeDeviceAddr     bool `json:"includeDeviceAddr,omitempty"`
	IncludeDeviceMetadata bool `json:"includeDeviceMetadata,omitempty"`
	IncludePei            bool `json:"includePei,omitempty"`
	IncludeSupi           bool `json:"includeSupi,omitempty"`
	IncludeImei           bool `json:"includeImei,omitempty"`
	IncludeImsi           bool `json:"includeImsi,omitempty"`
	IncludeIccid          bool `json:"includeIccid,omitempty"`
	IncludeDeviceId       bool `json:"includeDeviceId,omitempty"`
}

// DownlinkInfo describes the downlink communication configuration of the
// user transport (schema: DownlinkInfo).
type DownlinkInfo struct {
	DownlinkTopic string `json:"downlinkTopic,omitempty"`
	DevicePort    int    `json:"devicePort,omitempty"`
}

// TrafficRuleDescriptor describes a MEC traffic rule the IoT device is
// requested to be associated to (schema: TrafficRuleDescriptor, from
// ETSI GS MEC 010-2).
type TrafficRuleDescriptor struct {
	// Required
	TrafficRuleId string           `json:"trafficRuleId"`
	FilterType    string           `json:"filterType"` // "per FLOW" | "per PACKET"
	Priority      int              `json:"priority"`
	TrafficFilter []TrafficFilter  `json:"trafficFilter"`
	Action        string           `json:"action"` // "SEE_DESCRIPTION"

	// Optional
	DstInterface *InterfaceDescriptor `json:"dstInterface,omitempty"`
}

// TrafficFilter identifies specific flows/packets to be handled by the
// MEC host (schema: TrafficFilter).
type TrafficFilter struct {
	SrcAddress       []string `json:"srcAddress,omitempty"`
	DstAddress       []string `json:"dstAddress,omitempty"`
	SrcPort          []string `json:"srcPort,omitempty"`
	DstPort          []string `json:"dstPort,omitempty"`
	Protocol         []string `json:"protocol,omitempty"`
	Tag              []string `json:"tag,omitempty"`
	Uri              []string `json:"uri,omitempty"`
	PacketLabel      []string `json:"packetLabel,omitempty"`
	SrcTunnelAddress []string `json:"srcTunnelAddress,omitempty"`
	TgtTunnelAddress []string `json:"tgtTunnelAddress,omitempty"`
	SrcTunnelPort    []string `json:"srcTunnelPort,omitempty"`
	DstTunnelPort    []string `json:"dstTunnelPort,omitempty"`
	QCI              int      `json:"qCI,omitempty"`
	DSCP             int      `json:"dSCP,omitempty"`
	TC               int      `json:"tC,omitempty"`
}

// InterfaceDescriptor describes the destination interface for a traffic
// rule (schema: InterfaceDescriptor).
type InterfaceDescriptor struct {
	// Required
	InterfaceType string `json:"interfaceType"` // "TUNNEL" | "MAC" | "IP"

	// Optional
	TunnelInfo    *TunnelInfo `json:"tunnelInfo,omitempty"`
	SrcMACAddress string      `json:"srcMACAddress,omitempty"`
	DstMACAddress string      `json:"dstMACAddress,omitempty"`
	DstIPAddress  string      `json:"dstIPAddress,omitempty"`
}

// TunnelInfo describes GTP/GRE tunnel parameters (schema: TunnelInfo).
type TunnelInfo struct {
	// Required
	TunnelType        string `json:"tunnelType"` // "GTP-U" | "GRE"
	TunnelDstAddress  string `json:"tunnelDstAddress"`
	TunnelSrcAddress  string `json:"tunnelSrcAddress"`

	// Optional
	TunnelSpecificData string `json:"tunnelSpecificData,omitempty"`
}

// ProblemDetails is the standard error response body used for 400/403/404
// responses (schema: ProblemDetails).
type ProblemDetails struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Status   int    `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}