# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorStatusIndividualSubscriptionGET**](SensorStatusSubscriptionApi.md#SensorStatusIndividualSubscriptionGET) | **Get** /subscriptions/sensor_status/{subscriptionId} | The GET method is used to retrieve information about this subscription
[**SensorStatusIndividualSubscriptionGET**](SensorStatusSubscriptionApi.md#SensorStatusIndividualSubscriptionGET) | **Get** /subscriptions/sensor_status | The GET method is used to request information about the subscriptions related to sensor status for this requestor
[**SensorStatusSubscriptionDELETE**](SensorStatusSubscriptionApi.md#SensorStatusSubscriptionDELETE) | **Delete** /subscriptions/sensor_status/{subscriptionId} | The DELETE method is used to cancel the existing subscription
[**SensorStatusSubscriptionPUT**](SensorStatusSubscriptionApi.md#SensorStatusSubscriptionPUT) | **Put** /subscriptions/sensor_status/{subscriptionId} | The PUT method is used to update the existing subscription

# **SensorStatusIndividualSubscriptionGET**
> []SensorStatusSubscription SensorStatusIndividualSubscriptionGET(ctx, subscriptionId)
The GET method is used to retrieve information about this subscription

This method shall support the request and response data structures, and response codes, as specified in Tables 7.8.3.1-1 and 7.8.3.1-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Unique identifiers of the subscription | 

### Return type

[**[]SensorStatusSubscription**](SensorStatusSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorStatusIndividualSubscriptionGET**
> []SubscriptionLinkList SensorStatusIndividualSubscriptionGET(ctx, optional)
The GET method is used to request information about the subscriptions related to sensor status for this requestor

This method shall support the URI query parameter, request and response data structures, and response codes, as specified in Tables 7.7.3.1-1 and 7.7.3.1-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***SensorStatusSubscriptionApiSensorStatusIndividualSubscriptionGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a SensorStatusSubscriptionApiSensorStatusIndividualSubscriptionGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sensorIdentifier** | [**optional.Interface of []string**](string.md)| Unique identifiers of the sensors | 

### Return type

[**[]SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorStatusSubscriptionDELETE**
> []SubscriptionLinkList SensorStatusSubscriptionDELETE(ctx, subscriptionId)
The DELETE method is used to cancel the existing subscription

This method shall support the request and response data structures, and response codes, as specified in Tables 7.8.3.5-1 and 7.8.3.5-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| It uniquely identifies a subscription | 

### Return type

[**[]SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorStatusSubscriptionPUT**
> []SensorStatusSubscription SensorStatusSubscriptionPUT(ctx, body, subscriptionId)
The PUT method is used to update the existing subscription

This method shall support the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.8.3.2-1 and 7.8.3.2-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SensorStatusSubscriptionIdBody**](SensorStatusSubscriptionIdBody.md)| New SensorStatusSubscription is included as entity body of the request | 
  **subscriptionId** | **string**| It uniquely identifies the created individual application mobility service | 

### Return type

[**[]SensorStatusSubscription**](SensorStatusSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

