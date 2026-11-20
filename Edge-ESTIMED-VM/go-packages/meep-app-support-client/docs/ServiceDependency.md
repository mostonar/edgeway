# ServiceDependency

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**RequestedPermissions** | [**[]Object**](.md) | Requested permissions regarding the access of the application to the service. See clause 8.2 of ETSI GS MEC 009 [4]. The format of this attribute is left for the data model design stage. | [optional] [default to null]
**SerCategory** | [***CategoryRef**](CategoryRef.md) |  | [optional] [default to null]
**SerName** | **string** | The name of the service, for example, RNIS, LocationService, etc. | [default to null]
**SerTransportDependencies** | [**[]TransportDependency**](TransportDependency.md) | Indicates transport and serialization format dependencies of consuming the service. Defaults to REST + JSON if absent. See note. | [optional] [default to null]
**Version** | **string** | The version of the service. | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

