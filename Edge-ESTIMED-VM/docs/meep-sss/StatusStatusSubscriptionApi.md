# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorStatusSubscriptionPOST**](StatusStatusSubscriptionApi.md#SensorStatusSubscriptionPOST) | **Post** /subscriptions/sensor_status | The POST method is used to create a new subscription to sensor status notifications

# **SensorStatusSubscriptionPOST**
> []SensorStatusSubscription SensorStatusSubscriptionPOST(ctx, body)
The POST method is used to create a new subscription to sensor status notifications

This method shall support the request and response data structures, and response codes, as specified in Table 7.7.3.4-1

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsSensorStatusBody**](SubscriptionsSensorStatusBody.md)| The entity body in the request contains data type of the specific sensor status subscription that is to be created | 

### Return type

[**[]SensorStatusSubscription**](SensorStatusSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

