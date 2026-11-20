# {{classname}}

All URIs are relative to *https://localhost/sandboxname/service-apis/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ApplicationsSubscriptionDELETE**](AppSubscriptionsApi.md#ApplicationsSubscriptionDELETE) | **Delete** /{subscriberId}/subscriptions/{subscriptionId}  | Delete an existing CAPIF Event Subscription
[**ApplicationsSubscriptionPatch**](AppSubscriptionsApi.md#ApplicationsSubscriptionPatch) | **Patch** /{subscriberId}/subscriptions/{subscriptionId}  | Modify an existing CAPIF Event Subscription
[**ApplicationsSubscriptionsPOST**](AppSubscriptionsApi.md#ApplicationsSubscriptionsPOST) | **Post** /{subscriberId}/subscriptions | Create a new CAPIF Events Subscription
[**ApplicationsSubscriptionsPUT**](AppSubscriptionsApi.md#ApplicationsSubscriptionsPUT) | **Put** /{subscriberId}/subscriptions/{subscriptionId}  | The PUT method is used to update an existing subscription resource.

# **ApplicationsSubscriptionDELETE**
> ApplicationsSubscriptionDELETE(ctx, subscriberId, subscriptionId)
Delete an existing CAPIF Event Subscription

This endpoint deletes an existing CAPIF Event Subscription resource.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriberId** | **string**| This variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6).  | 
  **subscriptionId** | **string**| Represents a subscription to the notifications from the MEC platform. | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ApplicationsSubscriptionPatch**
> EventSubscription ApplicationsSubscriptionPatch(ctx, body, subscriberId, subscriptionId)
Modify an existing CAPIF Event Subscription

This endpoint modifies an existing CAPIF Event Subscription resource.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**EventSubscriptionPatch**](EventSubscriptionPatch.md)|  | 
  **subscriberId** | **string**| This variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6).  | 
  **subscriptionId** | **string**| Represents a subscription to the notifications from the MEC platform. | 

### Return type

[**EventSubscription**](EventSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ApplicationsSubscriptionsPOST**
> EventSubscription ApplicationsSubscriptionsPOST(ctx, body, subscriberId)
Create a new CAPIF Events Subscription

Create a new CAPIF Events Subscription resource.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**EventSubscription**](EventSubscription.md)|  | 
  **subscriberId** | **string**| This variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6).  | 

### Return type

[**EventSubscription**](EventSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ApplicationsSubscriptionsPUT**
> EventSubscription ApplicationsSubscriptionsPUT(ctx, body, subscriberId, subscriptionId)
The PUT method is used to update an existing subscription resource.

The susbcribing entity shall initiate the HTTP PUT request message and the CAPIF core function shall respond to the message.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**EventSubscription**](EventSubscription.md)|  | 
  **subscriberId** | **string**| This variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6).  | 
  **subscriptionId** | **string**| Represents a subscription to the notifications from the MEC platform. | 

### Return type

[**EventSubscription**](EventSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

