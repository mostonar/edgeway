# IotPlatformInfo

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**IotPlatformId** | **string** | Identifier of the IoT platform. | [default to null]
**UserTransportInfo** | [**[]MbTransportInfo**](MBTransportInfo.md) | Information about the user transport(s)  provided by the IoT platform. | [default to null]
**CustomServicesTransportInfo** | [**[]TransportInfo**](TransportInfo.md) | Transport enabling access to vendor-specific services provided by the IoT platform. The data type definition is as per ETSI GS MEC 011 [i.2]. | [optional] [default to null]
**Enabled** | **bool** | Indication whether the IoT platform is capable of providing user transports and vendor-specific services (TRUE) or not (FALSE). | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

