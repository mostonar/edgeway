# {{classname}}

All URIs are relative to *http://127.0.0.1:8081/sandbox_api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxMecServicesGET**](SandboxMECServicesApi.md#SandboxMecServicesGET) | **Get** /sandboxMecServices | get the list of the available MEC services
[**SandboxMecServicesPOST**](SandboxMECServicesApi.md#SandboxMecServicesPOST) | **Post** /sandboxMecServices/{mec_service_name} | select the MEC services to use

# **SandboxMecServicesGET**
> []SandboxMecServices SandboxMecServicesGET(ctx, )
get the list of the available MEC services

This method retrieves the list of the available MEC services.

### Required Parameters
This endpoint does not need any parameter.

### Return type

[**[]SandboxMecServices**](SandboxMecServices.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxMecServicesPOST**
> []SandboxMecServices SandboxMecServicesPOST(ctx, mecServiceName)
select the MEC services to use

This method selects the MEC service to be used.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **mecServiceName** | **string**| It uniquely identifies a MEC service to use | 

### Return type

[**[]SandboxMecServices**](SandboxMecServices.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

