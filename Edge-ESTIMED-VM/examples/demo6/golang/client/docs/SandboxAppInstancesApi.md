# {{classname}}

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxAppInstancesDELETE**](SandboxAppInstancesApi.md#SandboxAppInstancesDELETE) | **Delete** /sandboxAppInstances/{sandbox_name}/{app_instance_id} | Delete an existing application instance
[**SandboxAppInstancesGET**](SandboxAppInstancesApi.md#SandboxAppInstancesGET) | **Get** /sandboxAppInstances/{sandbox_name} | Get the list of the available application instance identifiers
[**SandboxAppInstancesPOST**](SandboxAppInstancesApi.md#SandboxAppInstancesPOST) | **Post** /sandboxAppInstances/{sandbox_name} | Create a new application instance identifier

# **SandboxAppInstancesDELETE**
> SandboxAppInstancesDELETE(ctx, sandboxName, appInstanceId)
Delete an existing application instance

This method removes an existing application instance

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 
  **appInstanceId** | **string**| It uniquely identifies a MEC application instance identifier | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxAppInstancesGET**
> []SandboxAppInstances SandboxAppInstancesGET(ctx, sandboxName)
Get the list of the available application instance identifiers

This method retrieves the list of the available application instance identifiers

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 

### Return type

[**[]SandboxAppInstances**](SandboxAppInstances.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxAppInstancesPOST**
> []ApplicationInfo SandboxAppInstancesPOST(ctx, body, sandboxName)
Create a new application instance identifier

This method creates a new application instance

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**ApplicationInfo**](ApplicationInfo.md)| Pet to add to the store | 
  **sandboxName** | **string**| Sandbox identifier | 

### Return type

[**[]ApplicationInfo**](ApplicationInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

