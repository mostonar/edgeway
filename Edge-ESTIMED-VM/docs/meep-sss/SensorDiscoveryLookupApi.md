# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorDiscoveryLookupGET**](SensorDiscoveryLookupApi.md#SensorDiscoveryLookupGET) | **Get** /queries/sensor_discovery | The GET method is used to query the available sensors

# **SensorDiscoveryLookupGET**
> []SensorDiscoveryInfo SensorDiscoveryLookupGET(ctx, optional)
The GET method is used to query the available sensors

This method shall support the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.3.3.1-1 and 7.3.3.1-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***SensorDiscoveryLookupApiSensorDiscoveryLookupGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a SensorDiscoveryLookupApiSensorDiscoveryLookupGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sensorInfo** | [**optional.Interface of SensorInfo**](.md)| Object containing the characteristics of the sensor(s) to be selected for the query | 

### Return type

[**[]SensorDiscoveryInfo**](SensorDiscoveryInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

