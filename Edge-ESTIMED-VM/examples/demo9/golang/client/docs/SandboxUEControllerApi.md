# {{classname}}

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxUeControllerGET**](SandboxUEControllerApi.md#SandboxUeControllerGET) | **Get** /sandboxUeController/{sandbox_name} | Get the list of the available UEs (e.g. \&quot;Stationary UE\&quot;)
[**SandboxUeControllerPATCH**](SandboxUEControllerApi.md#SandboxUeControllerPATCH) | **Patch** /sandboxUeController/{sandbox_name} | set the new value of the UE

# **SandboxUeControllerGET**
> []Ue SandboxUeControllerGET(ctx, sandboxName)
Get the list of the available UEs (e.g. \"Stationary UE\")

This method retrieves the list of the available available UEs.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 

### Return type

[**[]Ue**](UE.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxUeControllerPATCH**
> SandboxUeControllerPATCH(ctx, sandboxName, userEquipmentId, userEquipmentValue)
set the new value of the UE

This method sets the new value of the UE.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 
  **userEquipmentId** | **string**| User equipmenet identifier | 
  **userEquipmentValue** | **int32**| It uniquely identifies a UE to set the new value | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

