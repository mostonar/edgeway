# {{classname}}

All URIs are relative to */auth/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**Authenticate**](AuthApi.md#Authenticate) | **Get** /authenticate | Authenticate service request
[**Authorize**](AuthApi.md#Authorize) | **Get** /authorize | OAuth authorization response endpoint
[**Login**](AuthApi.md#Login) | **Get** /login | Initiate OAuth login procedure
[**LoginSupported**](AuthApi.md#LoginSupported) | **Get** /loginSupported | Check if login is supported
[**LoginUser**](AuthApi.md#LoginUser) | **Post** /login | Start a session
[**Logout**](AuthApi.md#Logout) | **Get** /logout | Terminate a session
[**TriggerWatchdog**](AuthApi.md#TriggerWatchdog) | **Post** /watchdog | Send heartbeat to watchdog

# **Authenticate**
> Authenticate(ctx, optional)
Authenticate service request

Authenticate & authorize microservice endpoint access

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***AuthApiAuthenticateOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a AuthApiAuthenticateOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **svc** | **optional.String**| Service requesting authentication | 
 **sbox** | **optional.String**| Sandbox name | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **Authorize**
> Authorize(ctx, optional)
OAuth authorization response endpoint

Redirect URI endpoint for OAuth authorization responses. Starts a user session.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***AuthApiAuthorizeOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a AuthApiAuthorizeOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **code** | **optional.String**| Temporary authorization code | 
 **state** | **optional.String**| User-provided random state | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **Login**
> Login(ctx, optional)
Initiate OAuth login procedure

Start OAuth login procedure with provider

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***AuthApiLoginOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a AuthApiLoginOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **provider** | **optional.String**| Oauth provider | 
 **sbox** | **optional.String**| Create Sandbox by default | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **LoginSupported**
> LoginSupported(ctx, )
Check if login is supported

Check if login is supported and whether session exists

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

# **LoginUser**
> Sandbox LoginUser(ctx, optional)
Start a session

Start a session after authenticating user

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***AuthApiLoginUserOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a AuthApiLoginUserOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **username** | **optional.**|  | 
 **password** | **optional.**|  | 

### Return type

[**Sandbox**](Sandbox.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/x-www-form-urlencoded
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **Logout**
> Logout(ctx, )
Terminate a session

Terminate a session

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

# **TriggerWatchdog**
> TriggerWatchdog(ctx, )
Send heartbeat to watchdog

Send heartbeat to watchdog to keep session alive

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

