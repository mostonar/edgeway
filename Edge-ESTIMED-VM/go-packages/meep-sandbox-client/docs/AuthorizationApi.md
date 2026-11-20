# {{classname}}

All URIs are relative to *http://127.0.0.1:8081/sandbox_api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**Login**](AuthorizationApi.md#Login) | **Post** /login | Initiate OAuth login procedure and creates MEC Sandbox
[**Logout**](AuthorizationApi.md#Logout) | **Post** /logout | Terminates User Session and User Sandbox

# **Login**
> Login(ctx, provider)
Initiate OAuth login procedure and creates MEC Sandbox

Redirect the browser to the provider login page and creates your MEC Sandbox on successful login and authorization

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **provider** | **string**| Oauth provider | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **Logout**
> Logout(ctx, )
Terminates User Session and User Sandbox

Terminates User Session and User Sandbox

### Required Parameters
This endpoint does not need any parameter.

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

