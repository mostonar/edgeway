# {{classname}}

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**Login**](AuthorizationApi.md#Login) | **Post** /login | Initiate OAuth login procedure and creates a MEC Sandbox instance
[**Logout**](AuthorizationApi.md#Logout) | **Post** /logout | Terminates User Session and delete the Sandbox instance

# **Login**
> Sandbox Login(ctx, provider)
Initiate OAuth login procedure and creates a MEC Sandbox instance

Initiate OAuth login procedure and creates a MEC Sandbox instance

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **provider** | **string**| Oauth provider | 

### Return type

[**Sandbox**](Sandbox.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **Logout**
> Logout(ctx, sandboxName)
Terminates User Session and delete the Sandbox instance

Terminates User Session and delete the Sandbox instance

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

