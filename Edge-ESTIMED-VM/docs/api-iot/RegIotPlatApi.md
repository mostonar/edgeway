# {{classname}}

All URIs are relative to *https://localhost/sandboxname/iots/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**RegisterediotplatformsByIdDELETE**](RegIotPlatApi.md#RegisterediotplatformsByIdDELETE) | **Delete** /registered_iot_platforms/{registeredIotPlatformId} | Deregister a IoT platform - see clause 5.3.6.
[**RegisterediotplatformsByIdGET**](RegIotPlatApi.md#RegisterediotplatformsByIdGET) | **Get** /registered_iot_platforms/{registeredIotPlatformId} | Discover native services and obtain information about user transports provided by a IoT platform - see clauses 5.3.3 and 5.4.2.
[**RegisterediotplatformsByIdPUT**](RegIotPlatApi.md#RegisterediotplatformsByIdPUT) | **Put** /registered_iot_platforms/{registeredIotPlatformId} | Update information about a IoT platform - see clause 5.3.5.
[**RegisterediotplatformsGET**](RegIotPlatApi.md#RegisterediotplatformsGET) | **Get** /registered_iot_platforms | Retrieve information about all IoT platforms - see clause 5.3.2.
[**RegisterediotplatformsPOST**](RegIotPlatApi.md#RegisterediotplatformsPOST) | **Post** /registered_iot_platforms | Register a IoT platform - see clause 5.3.4.

# **RegisterediotplatformsByIdDELETE**
> RegisterediotplatformsByIdDELETE(ctx, registeredIotPlatformId)
Deregister a IoT platform - see clause 5.3.6.

The DELETE method may be used by a service consumer to deregister a IoT platform. This method is typically used in the \"IoT platform deregistration\" procedure as described in clause 5.3.6.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **registeredIotPlatformId** | **string**| The unique identifier of the registered IoT platform. | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegisterediotplatformsByIdGET**
> InlineResponse2001 RegisterediotplatformsByIdGET(ctx, registeredIotPlatformId, optional)
Discover native services and obtain information about user transports provided by a IoT platform - see clauses 5.3.3 and 5.4.2.

The GET method may be used by a service consumer to obtain information about a IoT platform. This method is typically used in the \"IoT platform information request\" procedure as described in clause 5.3.3 and in the \"user transport query\" procedure as described in clause 5.4.2. 

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **registeredIotPlatformId** | **string**| The unique identifier of the registered IoT platform. | 
 **optional** | ***RegIotPlatApiRegisterediotplatformsByIdGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a RegIotPlatApiRegisterediotplatformsByIdGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **fields** | [**optional.Interface of []string**](string.md)| The list may contain one or more of the following attributes from the IotPlatformInfo data type: - userTransportInfo - customServiceTransportInfo  | 

### Return type

[**InlineResponse2001**](inline_response_200_1.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegisterediotplatformsByIdPUT**
> RegisteredIotPlatformsRegisteredIotPlatformIdBody RegisterediotplatformsByIdPUT(ctx, body, registeredIotPlatformId)
Update information about a IoT platform - see clause 5.3.5.

The PUT method may be used by a service consumer to update a IoT platform registration. This method is typically used in the \"IoT platform update\" procedure as described in clause 5.3.5.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**RegisteredIotPlatformsRegisteredIotPlatformIdBody**](RegisteredIotPlatformsRegisteredIotPlatformIdBody.md)| One or more updated attributes that are allowed to be changed are included in the IotPlatformInfo data structure in the payload body of the request. . | 
  **registeredIotPlatformId** | **string**| The unique identifier of the registered IoT platform. | 

### Return type

[**RegisteredIotPlatformsRegisteredIotPlatformIdBody**](registered_iot_platforms_registeredIotPlatformId_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegisterediotplatformsGET**
> []IotPlatformInfo RegisterediotplatformsGET(ctx, optional)
Retrieve information about all IoT platforms - see clause 5.3.2.

The GET method may be used by an authorized service consumer to retrieve the information of all currently registered IoT platforms. This method is typically used in the \"registered IoT platforms query\" procedure as described in clause 5.3.2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***RegIotPlatApiRegisterediotplatformsGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a RegIotPlatApiRegisterediotplatformsGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **fields** | [**optional.Interface of []string**](string.md)| The list shall contain the following attributes from the IotPlatformInfo data type: - iotPlatformId - enabled  | 

### Return type

[**[]IotPlatformInfo**](IotPlatformInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegisterediotplatformsPOST**
> RegisteredIotPlatformsBody RegisterediotplatformsPOST(ctx, body)
Register a IoT platform - see clause 5.3.4.

The POST method may be used by a service consumer to register a new IoT platform. This method is typically used in the \"IoT platform registration\" procedure as described in clause 5.3.4.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**RegisteredIotPlatformsBody**](RegisteredIotPlatformsBody.md)| Payload body in the request contains the information associated to the IoT platform to be registered. | 

### Return type

[**RegisteredIotPlatformsBody**](registered_iot_platforms_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

