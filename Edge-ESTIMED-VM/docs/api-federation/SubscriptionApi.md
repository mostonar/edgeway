# {{classname}}

All URIs are relative to *https://localhost/sandboxname/fed_enablement/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SubscriptionDELETE**](SubscriptionApi.md#SubscriptionDELETE) | **Delete** /subscriptions/{subscriptionId} | Cancel an existing subscription
[**SubscriptionGET**](SubscriptionApi.md#SubscriptionGET) | **Get** /subscriptions/{subscriptionId} | Retrieve information on current specific subscription
[**SubscriptionPOST**](SubscriptionApi.md#SubscriptionPOST) | **Post** /subscriptions | Create a new subscription
[**SubscriptionPUT**](SubscriptionApi.md#SubscriptionPUT) | **Put** /subscriptions/{subscriptionId} | Modify an existing subscription
[**SubscriptionsGET**](SubscriptionApi.md#SubscriptionsGET) | **Get** /subscriptions | Retrieve information on subscriptions for notifications

# **SubscriptionDELETE**
> SubscriptionDELETE(ctx, subscriptionId)
Cancel an existing subscription

Cancels an existing subscription, identified by its self-referring URI  returned on creation (initial POST)

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Id, specifically the \\\&quot;self\\\&quot; returned in the subscription  request\&quot; | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionGET**
> SystemUpdateNotificationSubscription SubscriptionGET(ctx, subscriptionId)
Retrieve information on current specific subscription

Queries information about an existing subscription, identified by its  self-referring URI returned on creation\"

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Id, specifically the \\\&quot;self\\\&quot; returned in the subscription  request | 

### Return type

[**SystemUpdateNotificationSubscription**](SystemUpdateNotificationSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionPOST**
> SystemUpdateNotificationSubscription SubscriptionPOST(ctx, body)
Create a new subscription

Creates a new subscription to to MEF

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SystemUpdateNotificationSubscription**](SystemUpdateNotificationSubscription.md)| Subscription to be created | 

### Return type

[**SystemUpdateNotificationSubscription**](SystemUpdateNotificationSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionPUT**
> SystemUpdateNotificationSubscription SubscriptionPUT(ctx, body, subscriptionId)
Modify an existing subscription

Updates an existing subscription, identified by its self-referring URI returned on creation

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SystemUpdateNotificationSubscription**](SystemUpdateNotificationSubscription.md)| Subscription to be modified | 
  **subscriptionId** | **string**| Subscription Id, specifically the \\\&quot;self\\\&quot; returned in the subscription  request\&quot; | 

### Return type

[**SystemUpdateNotificationSubscription**](SystemUpdateNotificationSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionsGET**
> SubscriptionLinkList SubscriptionsGET(ctx, optional)
Retrieve information on subscriptions for notifications

Queries information on subscriptions for notifications

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***SubscriptionApiSubscriptionsGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a SubscriptionApiSubscriptionsGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **subscriptionType** | **optional.String**| Query parameter is used to filter a specific subscription type | 
 **systemId** | **optional.String**| Identifier of the MEC system | 

### Return type

[**SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

