# WaiApi

All URIs are relative to *https://localhost/sandboxname/wai/v2*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ApInfoGET**](WaiApi.md#ApInfoGET) | **Get** /queries/ap/ap_information | Retrieve information on existing Access Points
[**Mec011AppTerminationPOST**](WaiApi.md#Mec011AppTerminationPOST) | **Post** /notifications/mec011/appTermination | MEC011 Application Termination notification for self termination
[**StaInfoGET**](WaiApi.md#StaInfoGET) | **Get** /queries/sta/sta_information | Retrieve information on existing Stations
[**SubscriptionLinkListSubscriptionsGET**](WaiApi.md#SubscriptionLinkListSubscriptionsGET) | **Get** /subscriptions | Retrieve information on subscriptions for notifications
[**SubscriptionsDELETE**](WaiApi.md#SubscriptionsDELETE) | **Delete** /subscriptions/{subscriptionId} | Cancel an existing subscription
[**SubscriptionsGET**](WaiApi.md#SubscriptionsGET) | **Get** /subscriptions/{subscriptionId} | Retrieve information on current specific subscription
[**SubscriptionsPOST**](WaiApi.md#SubscriptionsPOST) | **Post** /subscriptions | Create a new subscription
[**SubscriptionsPUT**](WaiApi.md#SubscriptionsPUT) | **Put** /subscriptions/{subscriptionId} | Modify an existing subscription


# **ApInfoGET**
> []ApInfo ApInfoGET(ctx, optional)
Retrieve information on existing Access Points

Queries information about existing WLAN Access Points

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **filter** | **String**| Attribute-based filtering expression according to clause 6.19 of ETSI GS MEC 009. . | [optional] [default to null]
 **all\_fields** | **String**| Include all complex attributes in the response. See clause 6.18 of ETSI GS MEC 009 for details. | [optional] [default to null]
 **fields** | [**List**](../Models/String.md)| Complex attributes to be included into the response. See clause 6.18 of ETSI GS MEC 009 for details. | [optional] [default to null]
 **exclude\_fields** | [**List**](../Models/String.md)| Complex attributes to be excluded from the response. See clause 6.18 of ETSI GS MEC 009 for details. | [optional] [default to null]
 **exclude\_default** | [**List**](../Models/String.md)| Indicates to exclude the following complex attributes from the response. See clause 6.18 of ETSI GS MEC 009 for details. The following attributes shall be excluded from the structure in the response body if this parameter is provided, or none of the parameters \&quot;all_fields\&quot;, \&quot;fields\&quot;, \&quot;exclude_fields\&quot;, \&quot;exclude_default\&quot; are provided: Not applicable | [optional] [default to null]

### Return type

[**[]ApInfo**](../Models/ApInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **Mec011AppTerminationPOST**
> Mec011AppTerminationPOST(ctx, body)
MEC011 Application Termination notification for self termination

Terminates itself.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **AppTerminationNotification** | [**AppTerminationNotification**](../Models/AppTerminationNotification.md)| Termination notification details | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **StaInfoGET**
> []StaInfo StaInfoGET(ctx, optional)
Retrieve information on existing Stations

Queries information about existing WLAN stations

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | **StaInfoGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a StaInfoGETOpts struct

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **filter** | **String**| Attribute-based filtering expression according to clause 6.19 of ETSI GS MEC 009. . | [optional] [default to null]
 **all\_fields** | **String**| Include all complex attributes in the response. See clause 6.18 of ETSI GS MEC 009 for details. | [optional] [default to null]
 **fields** | [**List**](../Models/String.md)| Complex attributes to be included into the response. See clause 6.18 of ETSI GS MEC 009 for details. | [optional] [default to null]
 **exclude\_fields** | [**List**](../Models/String.md)| Complex attributes to be excluded from the response. See clause 6.18 of ETSI GS MEC 009 for details. | [optional] [default to null]
 **exclude\_default** | [**List**](../Models/String.md)| Indicates to exclude the following complex attributes from the response. See clause 6.18 of ETSI GS MEC 009 for details. The following attributes shall be excluded from the structure in the response body if this parameter is provided, or none of the parameters \&quot;all_fields\&quot;, \&quot;fields\&quot;, \&quot;exclude_fields\&quot;, \&quot;exclude_default\&quot; are provided: Not applicable | [optional] [default to null]

### Return type

[**[]StaInfo**](../Models/StaInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionLinkListSubscriptionsGET**
> SubscriptionLinkList SubscriptionLinkListSubscriptionsGET(ctx, optional)
Retrieve information on subscriptions for notifications

Queries information on subscriptions for notifications

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | **SubscriptionLinkListSubscriptionsGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a SubscriptionLinkListSubscriptionsGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **subscriptionType** | **optional.String**| Filter on a specific subscription type. Permitted values: assoc_sta, sta_data_rate, measure_report. | 

### Return type

[**SubscriptionLinkList**](../Models/SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionsDELETE**
> SubscriptionsDELETE(ctx, subscriptionId)
Cancel an existing subscription

Cancels an existing subscription, identified by its self-referring URI returned on creation (initial POST)

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Id, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionsGET**
> InlineSubscription SubscriptionsGET(ctx, subscriptionId)
Retrieve information on current specific subscription

Queries information about an existing subscription, identified by its self-referring URI returned on creation (initial POST)

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Id, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**InlineSubscription**](../Models/InlineSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionsPOST**
> InlineSubscription SubscriptionsPOST(ctx, body)
Create a new subscription

Creates a new subscription to WLAN Access Information notifications

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**InlineSubscription**](InlineSubscription.md)| Subscription to be created | 

### Return type

[**InlineSubscription**](../Models/InlineSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubscriptionsPUT**
> InlineSubscription SubscriptionsPUT(ctx, body, subscriptionId)
Modify an existing subscription

Updates an existing subscription, identified by its self-referring URI returned on creation (initial POST)

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**InlineSubscription**](InlineSubscription.md)| Subscription to be modified | 
  **subscriptionId** | **string**| Subscription Id, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**InlineSubscription**](../Models/InlineSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

