# {{classname}}

All URIs are relative to *https://localhost/sandboxname/sens/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SensorDiscoveryIndividualSubscriptionGET**](SensorDiscoverySubscriptionApi.md#SensorDiscoveryIndividualSubscriptionGET) | **Get** /subscriptions/sensor_discovery/{subscriptionId} | The GET method is used to request information about the subscriptions related to sensor status for this requestor
[**SensorDiscoverySubscriptionDELETE**](SensorDiscoverySubscriptionApi.md#SensorDiscoverySubscriptionDELETE) | **Delete** /subscriptions/sensor_discovery/{subscriptionId} | The DELETE method is used to cancel the existing subscription. Cancellation can be made by deleting the resource that represents existing sensor discovery subscription
[**SensorDiscoverySubscriptionGET**](SensorDiscoverySubscriptionApi.md#SensorDiscoverySubscriptionGET) | **Get** /subscriptions/sensor_discovery | The GET method is used to request information about the subscriptions related to sensor discovery for this requestor
[**SensorDiscoverySubscriptionPOST**](SensorDiscoverySubscriptionApi.md#SensorDiscoverySubscriptionPOST) | **Post** /subscriptions/sensor_discovery | The POST method is used to create a new subscription to sensor discovery notifications
[**SensorDiscoverySubscriptionPUT**](SensorDiscoverySubscriptionApi.md#SensorDiscoverySubscriptionPUT) | **Put** /subscriptions/sensor_discovery/{subscriptionId} | The POST method is used to create a new subscription to sensor discovery notifications

# **SensorDiscoveryIndividualSubscriptionGET**
> []SubscriptionLinkList SensorDiscoveryIndividualSubscriptionGET(ctx, subscriptionId)
The GET method is used to request information about the subscriptions related to sensor status for this requestor

This method shall support the URI query parameter, request and response data structures, and response codes, as specified in Tables 7.7.3.1-1 and 7.7.3.1-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Unique identifiers of the subscription | 

### Return type

[**[]SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorDiscoverySubscriptionDELETE**
> []SubscriptionLinkList SensorDiscoverySubscriptionDELETE(ctx, subscriptionId)
The DELETE method is used to cancel the existing subscription. Cancellation can be made by deleting the resource that represents existing sensor discovery subscription

This method shall support the request and response data structures, and response codes, as specified in Tables 7.5.3.5-1 and 7.5.3.5-2

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| It uniquely identifies the created individual application mobility service | 

### Return type

[**[]SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorDiscoverySubscriptionGET**
> []SubscriptionLinkList SensorDiscoverySubscriptionGET(ctx, optional)
The GET method is used to request information about the subscriptions related to sensor discovery for this requestor

This method shall support the URI query parameter, request and response data structures, and response codes, as specified in Tables 7.4.3.1-1 and 7.4.3.1-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***SensorDiscoverySubscriptionApiSensorDiscoverySubscriptionGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a SensorDiscoverySubscriptionApiSensorDiscoverySubscriptionGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **subscriptionId** | **optional.String**| Object containing the characteristics of the sensor(s) to be selected for the query | 

### Return type

[**[]SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorDiscoverySubscriptionPOST**
> []SensorDiscoveryEventSubscription SensorDiscoverySubscriptionPOST(ctx, body)
The POST method is used to create a new subscription to sensor discovery notifications

This method shall support the request and response data structures, and response codes, as specified in Table 7.4.3.4-1

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsSensorDiscoveryBody**](SubscriptionsSensorDiscoveryBody.md)| This type represents a subscription to notifications from the Sensor-sharing Service regarding events related to changes in sensor information | 

### Return type

[**[]SensorDiscoveryEventSubscription**](SensorDiscoveryEventSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SensorDiscoverySubscriptionPUT**
> []SubscriptionLinkList SensorDiscoverySubscriptionPUT(ctx, body, subscriptionId)
The POST method is used to create a new subscription to sensor discovery notifications

This method shall support the request and response data structures, and response codes, as specified in Table 7.4.3.4-1

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SensorDiscoverySubscriptionIdBody**](SensorDiscoverySubscriptionIdBody.md)| New SensorDiscoveryEventSubscription is included as entity body of the request | 
  **subscriptionId** | **string**| It uniquely identifies the created individual application mobility service | 

### Return type

[**[]SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

