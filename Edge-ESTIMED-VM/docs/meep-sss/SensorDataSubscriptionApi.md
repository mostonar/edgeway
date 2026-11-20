# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorDataIndividualSubscriptionGET**](SensorDataSubscriptionApi.md#SensorDataIndividualSubscriptionGET) | **Get** /queries/status_data/{subscriptionId} | The GET method is used to retrieve information about this subscription
[**SensorDataSubscriptionDELETE**](SensorDataSubscriptionApi.md#SensorDataSubscriptionDELETE) | **Delete** /queries/status_data/{subscriptionId} | The DELETE method is used to cancel the existing subscription
[**SensorDataSubscriptionGET**](SensorDataSubscriptionApi.md#SensorDataSubscriptionGET) | **Get** /subscriptions/sensor_data | The GET method is used to request information about the subscriptions related to sensor data for this requestor
[**SensorDataSubscriptionPOST**](SensorDataSubscriptionApi.md#SensorDataSubscriptionPOST) | **Post** /subscriptions/sensor_data | The POST method is used to create a new subscription to sensor status notifications
[**SensorDataSubscriptionPUT**](SensorDataSubscriptionApi.md#SensorDataSubscriptionPUT) | **Put** /queries/status_data/{subscriptionId} | The PUT method is used to update the existing subscription

# **SensorDataIndividualSubscriptionGET**
> []SensorDataSubscription SensorDataIndividualSubscriptionGET(ctx, subscriptionId)
The GET method is used to retrieve information about this subscription

This method shall support the request and response data structures, and response codes, as specified in Tables 7.11.3.1-1 and 7.11.3.1-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Unique identifiers of a subscription | 

### Return type

[**[]SensorDataSubscription**](SensorDataSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorDataSubscriptionDELETE**
> []SubscriptionLinkList SensorDataSubscriptionDELETE(ctx, subscriptionId)
The DELETE method is used to cancel the existing subscription

This method shall support the request and response data structures, and response codes, as specified in Tables 7.11.3.5-1 and 7.11.3.5-2

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

# **SensorDataSubscriptionGET**
> []SubscriptionLinkList SensorDataSubscriptionGET(ctx, sensorIdentifier)
The GET method is used to request information about the subscriptions related to sensor data for this requestor

This method shall support the URI query parameter, request and response data structures, and response codes, as specified in Tables 7.10.3.1-1 and 7.10.3.1-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sensorIdentifier** | [**[]string**](string.md)| Unique identifiers of the sensors | 

### Return type

[**[]SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorDataSubscriptionPOST**
> []SensorStatusSubscription SensorDataSubscriptionPOST(ctx, body)
The POST method is used to create a new subscription to sensor status notifications

This method shall support the request and response data structures, and response codes, as specified in Table 7.7.3.4-1

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsSensorDataBody**](SubscriptionsSensorDataBody.md)| The entity body in the request contains data type of the specific sensor status subscription that is to be created | 

### Return type

[**[]SensorStatusSubscription**](SensorStatusSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorDataSubscriptionPUT**
> []SensorDataSubscription SensorDataSubscriptionPUT(ctx, body, subscriptionId)
The PUT method is used to update the existing subscription

This method shall support the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.11.3.2-1 and 7.11.3.2-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SensorDataSubscriptionIdBody**](SensorDataSubscriptionIdBody.md)| New SensorDataSubscription is included as entity body of the request | 
  **subscriptionId** | **string**| Unique identifiers of a subscription | 

### Return type

[**[]SensorDataSubscription**](SensorDataSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

