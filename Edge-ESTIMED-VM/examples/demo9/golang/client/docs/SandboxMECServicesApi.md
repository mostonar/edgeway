# {{classname}}

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxMecServicesGET**](SandboxMECServicesApi.md#SandboxMecServicesGET) | **Get** /sandboxMecServices/{sandbox_name} | Get the list of the available MEC services

# **SandboxMecServicesGET**
> []SandboxMecServices SandboxMecServicesGET(ctx, sandboxName)
Get the list of the available MEC services

This method retrieves the list of the available MEC services.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 

### Return type

[**[]SandboxMecServices**](SandboxMecServices.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

