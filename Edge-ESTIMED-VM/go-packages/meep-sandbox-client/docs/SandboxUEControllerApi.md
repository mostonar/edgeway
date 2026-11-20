# {{classname}}

All URIs are relative to *http://127.0.0.1:8081/sandbox_api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxUeControllerGET**](SandboxUEControllerApi.md#SandboxUeControllerGET) | **Get** /sandboxUeController | get the list of the available UEs (e.g. \&quot;Stationary UE\&quot;)
[**SandboxUeControllerPATCH**](SandboxUEControllerApi.md#SandboxUeControllerPATCH) | **Patch** /sandboxUeController/{user_equipment_id} | set the new value of the UE

# **SandboxUeControllerGET**
> []Ue SandboxUeControllerGET(ctx, )
get the list of the available UEs (e.g. \"Stationary UE\")

This method retrieves the list of the available available UEs.

### Required Parameters
This endpoint does not need any parameter.

### Return type

[**[]Ue**](UE.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxUeControllerPATCH**
> SandboxUeControllerPATCH(ctx, userEquipmentId, userEquipmentValue)
set the new value of the UE

This method sets the new value of the UE.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **userEquipmentId** | **string**| Network scenario to be used | 
  **userEquipmentValue** | **int32**| It uniquely identifies a UE to set the new value | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

