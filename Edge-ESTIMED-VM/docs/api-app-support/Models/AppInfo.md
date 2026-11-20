# AppInfo

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**AppName** | **string** | Name of the application. It shall be consistent with the appName in the AppD, if an AppD is available. | [default to null]
**AppProvider** | **string** | Provider of the application. It shall be consistent with the appProvider in the AppD, if an AppD is available. See note 1. | [optional] [default to null]
**AppCategory** | [***CategoryRef**](CategoryRef.md) |  | [optional] [default to null]
**AppDId** | **string** | The application descriptor identifier. It is managed by the application provider to identify the application descriptor in a globally unique way. Shall be present if the application instance is instantiated by the MEC Management. | [optional] [default to null]
**AppInstanceId** | **string** | Identifier of the application instance. Shall be present if the application instance is instantiated by the MEC Management. | [optional] [default to null]
**Endpoint** | [***OneOfAppInfoEndpoint**](OneOfAppInfoEndpoint.md) | This type represents information about a transport endpoint | [optional] [default to null]
**AppServiceRequired** | [**[]ServiceDependency**](ServiceDependency.md) | Describes services a MEC application requires to run. ServiceDependency is defined in ETSI GS MEC 010-2 [4]. It shall not be provided if an AppD is available. | [optional] [default to null]
**AppServiceOptional** | [**[]ServiceDependency**](ServiceDependency.md) | Describes services a MEC application may use if available. ServiceDependency is defined in ETSI GS MEC 010-2 [4]. It shall not be provided if an AppD is available. | [optional] [default to null]
**AppFeatureRequired** | [**[]FeatureDependency**](FeatureDependency.md) | Describes features a MEC application requires to run. FeatureDependency is defined in ETSI GS MEC 010-2 [4]. It shall not be provided if an AppD is available. | [optional] [default to null]
**AppFeatureOptional** | [**[]FeatureDependency**](FeatureDependency.md) | Describes features a MEC application may use if available. FeatureDependency is defined in ETSI GS MEC 010-2 [4]. It shall not be provided if an AppD is available. | [optional] [default to null]
**IsInsByMec** | **bool** | Indicate whether the application instance is instantiated by the MEC Management. Default to FALSE if absent. | [optional] [default to null]
**AppProfile** | [***AppProfile**](AppProfile.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

