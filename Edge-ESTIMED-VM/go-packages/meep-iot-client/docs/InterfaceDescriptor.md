# InterfaceDescriptor

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**InterfaceType** | **string** | Type of interface: TUNNEL, MAC, IP, etc. | [default to null]
**TunnelInfo** | [***TunnelInfo**](TunnelInfo.md) |  | [optional] [default to null]
**SrcMACAddress** | **string** | If the interface type is MAC, the source address identifies the MAC address of the interface. | [optional] [default to null]
**DstMACAddress** | **string** | If the interface type is MAC, the destination address identifies the MAC address of the destination. Only used for dstInterface. | [optional] [default to null]
**DstIPAddress** | **string** | If the interface type is IP, the destination address identifies the IP address of the destination. Only used for dstInterface. | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

