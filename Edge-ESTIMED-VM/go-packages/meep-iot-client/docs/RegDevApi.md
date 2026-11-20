# {{classname}}

All URIs are relative to *https://localhost/sandboxname/iots/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**RegistereddevicesByIdDELETE**](RegDevApi.md#RegistereddevicesByIdDELETE) | **Delete** /registered_devices/{registeredDeviceId} | Deregister a device - see clause 5.2.6.
[**RegistereddevicesByIdGET**](RegDevApi.md#RegistereddevicesByIdGET) | **Get** /registered_devices/{registeredDeviceId} | Retrieve information about a device - see clause 5.2.4.
[**RegistereddevicesByIdPUT**](RegDevApi.md#RegistereddevicesByIdPUT) | **Put** /registered_devices/{registeredDeviceId} | Update information about a  device including its association to a valid traffic rule - see clauses 5.2.5 and 5.4.3.
[**RegistereddevicesGET**](RegDevApi.md#RegistereddevicesGET) | **Get** /registered_devices | Retrieve information about all devices with a valid traffic rule association - see clause 5.2.2.
[**RegistereddevicesPOST**](RegDevApi.md#RegistereddevicesPOST) | **Post** /registered_devices | Register a new device - see clause 5.2.3.

# **RegistereddevicesByIdDELETE**
> RegistereddevicesByIdDELETE(ctx, registeredDeviceId)
Deregister a device - see clause 5.2.6.

The DELETE method may be used by a service consumer to deregister a device. This method is typically used in the \"device deregistration\" procedure as described in clause 5.2.6.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **registeredDeviceId** | **string**| The unique identifier of the registered device. | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegistereddevicesByIdGET**
> RegisteredDevicesBody RegistereddevicesByIdGET(ctx, registeredDeviceId)
Retrieve information about a device - see clause 5.2.4.

The GET method may be used by a service consumer to retrieve information about a device. This method is typically used in the \"device registration query\" procedure as described in clause 5.2.4.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **registeredDeviceId** | **string**| The unique identifier of the registered device. | 

### Return type

[**RegisteredDevicesBody**](registered_devices_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegistereddevicesByIdPUT**
> RegisteredDevicesRegisteredDeviceIdBody RegistereddevicesByIdPUT(ctx, body, registeredDeviceId)
Update information about a  device including its association to a valid traffic rule - see clauses 5.2.5 and 5.4.3.

The PUT method may be used by a service consumer to update a device registration including its association to a valid traffic rule. This method is typically used in the \"device registration update\" procedure as described in clause 5.2.5 and in the \"user transport assignment\" procedure as described in clause 5.4.3.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**RegisteredDevicesRegisteredDeviceIdBody**](RegisteredDevicesRegisteredDeviceIdBody.md)| One or more updated attributes that are allowed to be changed are included in the DeviceInfo data structure in the payload body of the request. . | 
  **registeredDeviceId** | **string**| The unique identifier of the registered device. | 

### Return type

[**RegisteredDevicesRegisteredDeviceIdBody**](registered_devices_registeredDeviceId_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegistereddevicesGET**
> InlineResponse200 RegistereddevicesGET(ctx, optional)
Retrieve information about all devices with a valid traffic rule association - see clause 5.2.2.

The GET method may be used by a service consumer to retrieve information about all devices with a valid traffic rule association. This method is typically used in the \"registered devices query\" procedure as described in clause 5.2.2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***RegDevApiRegistereddevicesGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a RegDevApiRegistereddevicesGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **filter** | **optional.String**| Attribute-based filtering parameters according to ETSI GS MEC 009 [i.1]. The API producer shall support receiving the following filtering parameters as part of the URI query string: \&quot;(eq,enabled,TRUE)\&quot;.  | 
 **fields** | [**optional.Interface of []string**](string.md)| The list may contain one or more of the following attributes from the DeviceInfo data type: - deviceMetadata - gpsi - msisdn - deviceId - requestedMecTrafficRule - requestedIotPlatformId - requestedUserTransportId  | 

### Return type

[**InlineResponse200**](inline_response_200.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **RegistereddevicesPOST**
> RegisteredDevicesBody RegistereddevicesPOST(ctx, body)
Register a new device - see clause 5.2.3.

The POST method may be used by a service consumer to register a new device. This method is typically used in the \"device registration\" procedure as described in clause 5.2.3.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**RegisteredDevicesBody**](RegisteredDevicesBody.md)| Payload body in the request contains the information associated to the IoT device to be registered. | 

### Return type

[**RegisteredDevicesBody**](registered_devices_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

