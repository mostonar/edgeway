# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorMgmtPUT**](ManagementnApi.md#SensorMgmtPUT) | **Put** /sensor_management | The PUT method is used to update the value of the characteristics that can be set on a sensor or on a group of sensors

# **SensorMgmtPUT**
> []SensorCharacteristic SensorMgmtPUT(ctx, body)
The PUT method is used to update the value of the characteristics that can be set on a sensor or on a group of sensors

This method shall support the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.12.3.2-1 and 7.12.3.2-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**[]SensorCharacteristic**](SensorCharacteristic.md)| The list of sensor characteristics to be updated, with the new values to set, is included as entity body of the request | 

### Return type

[**[]SensorCharacteristic**](SensorCharacteristic.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

