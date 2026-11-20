# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorStatusLookupGET**](SensorStatusLookupApi.md#SensorStatusLookupGET) | **Get** /queries/sensor_status | The GET method is used to query the available status

# **SensorStatusLookupGET**
> []SensorStatusInfo SensorStatusLookupGET(ctx, sensorIdentifier)
The GET method is used to query the available status

This method shall support the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.6.3.1-1 and 7.6.3.1-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sensorIdentifier** | [**[]string**](string.md)| It uniquely identifies the created individual application mobility service | 

### Return type

[**[]SensorStatusInfo**](SensorStatusInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

