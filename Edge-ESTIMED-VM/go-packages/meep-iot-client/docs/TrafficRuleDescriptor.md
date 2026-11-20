# TrafficRuleDescriptor

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**TrafficRuleId** | **string** | Identifies the traffic rule. | [default to null]
**FilterType** | **string** | Definition of filter type: per FLOW or PACKET  If it is per FLOW, the filter matches upstream (e.g. UE-&gt;EPC) packets and downstream (e.g. EPC-&gt;UE) packets are handled by the same context.  | [default to null]
**Priority** | **int32** | Priority of this traffic rule within the range 0 to 255. If traffic rule conflicts, the one with higher priority take precedence. See note 1. | [default to null]
**TrafficFilter** | [**[]TrafficFilter**](TrafficFilter.md) | The filter used to identify specific flow/packets that need to be handled by the MEC host. | [default to null]
**Action** | **string** | Identifies the action of the MEC host data plane, when a packet matches the trafficFilter, the example actions include: DROP, FORWARD_DECAPSULATED, FORWARD_ENCAPSULATED, PASSTHROUGH, DUPLICATE_DECAPSULATED, DUPLICATE_ENCAPSULATED  | [default to null]
**DstInterface** | [***InterfaceDescriptor**](InterfaceDescriptor.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

