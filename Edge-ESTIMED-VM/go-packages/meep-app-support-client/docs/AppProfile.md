# AppProfile

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**EasId** | **string** | The identifier of the EAS | [default to null]
**EndPt** | [***OneOfAppProfileEndPt**](OneOfAppProfileEndPt.md) | The version of the feature. | [default to null]
**AcIds** | **[]string** | Identities of the Application Clients that can be served by the EAS | [default to null]
**ProvId** | **string** | Identifier of the ASP that provides the EAS. | [optional] [default to null]
**Type_** | **string** | The category or type of EAS. | [optional] [default to null]
**Scheds** | **[]string** | The availability schedule of the EAS. | [default to null]
**SvcArea** | **string** | The list of geographical and topological areas that the EAS serves. ACs in the UE that are outside the area will not be served. | [optional] [default to null]
**SvcKpi** | **string** | Service characteristics provided by the EAS. | [optional] [default to null]
**PermLvl** | **[]string** | level of service permissions supported by the EAS. | [default to null]
**EasFeats** | **[]string** | Service features supported by the EAS. | [default to null]
**SvcContSupp** | **[]string** | The ACR scenarios supported by the EAS for service continuity. | [default to null]
**AppLocs** | **[]string** | List of DNAI(s) and the N6 traffic information associated with the EAS. | [default to null]
**AvlRep** | **int32** | The period indicating to the EES, how often the EES needs to check the EAS&#x27;s availability after a successful registration. | [optional] [default to null]
**Status** | **string** | EAS status information.  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

