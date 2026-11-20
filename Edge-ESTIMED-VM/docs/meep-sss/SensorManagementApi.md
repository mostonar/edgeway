# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorMgmtGET**](SensorManagementApi.md#SensorMgmtGET) | **Get** /sensor_management | The GET method is used to receive the sensor characteristics that can be modified on specific sensors

# **SensorMgmtGET**
> []SensorCharacteristic SensorMgmtGET(ctx, sensorIdentifier)
The GET method is used to receive the sensor characteristics that can be modified on specific sensors

This method shall support the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.12.3.1-1 and 7.12.3.1-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sensorIdentifier** | [**[]string**](string.md)| It uniquely identifies the created individual application mobility service | 

### Return type

[**[]SensorCharacteristic**](SensorCharacteristic.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

