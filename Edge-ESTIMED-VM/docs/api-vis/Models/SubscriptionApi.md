# {{classname}}

All URIs are relative to *https://localhost/vis/v2*

Method | HTTP request | Description
------------- | ------------- | -------------
[**IndividualSubscriptionDELETE**](SubscriptionApi.md#IndividualSubscriptionDELETE) | **Delete** /subscriptions/{subscriptionId} | Used to cancel the existing subscription.
[**IndividualSubscriptionGET**](SubscriptionApi.md#IndividualSubscriptionGET) | **Get** /subscriptions/{subscriptionId} | Retrieve information about this subscription.
[**IndividualSubscriptionPUT**](SubscriptionApi.md#IndividualSubscriptionPUT) | **Put** /subscriptions/{subscriptionId} | Used to update the existing subscription.
[**SubGET**](SubscriptionApi.md#SubGET) | **Get** /subscriptions | Request information about the subscriptions for this requestor.
[**SubPOST**](SubscriptionApi.md#SubPOST) | **Post** /subscriptions |  create a new subscription to VIS notifications.

# **IndividualSubscriptionDELETE**
> IndividualSubscriptionDELETE(ctx, subscriptionId)
Used to cancel the existing subscription.

Used to cancel the existing subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Refers to created subscription, where the VIS API allocates a unique resource name for this subscription | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **IndividualSubscriptionGET**
> InlineResponse200 IndividualSubscriptionGET(ctx, subscriptionId)
Retrieve information about this subscription.

Retrieve information about this subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Refers to created subscription, where the VIS API allocates a unique resource name for this subscription | 

### Return type

[**InlineResponse200**](inline_response_200.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **IndividualSubscriptionPUT**
> SubscriptionsSubscriptionIdBody IndividualSubscriptionPUT(ctx, body, subscriptionId)
Used to update the existing subscription.

Used to update the existing subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsSubscriptionIdBody**](SubscriptionsSubscriptionIdBody.md)|  | 
  **subscriptionId** | **string**| Refers to created subscription, where the VIS API allocates a unique resource name for this subscription | 

### Return type

[**SubscriptionsSubscriptionIdBody**](subscriptions_subscriptionId_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubGET**
> SubscriptionLinkList SubGET(ctx, optional)
Request information about the subscriptions for this requestor.

Request information about the subscriptions for this requestor.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***SubscriptionApiSubGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a SubscriptionApiSubGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **subscriptionType** | **optional.String**| Query parameter to filter on a specific subscription type. Permitted values:  prov_chg_uu_uni: provisioning information change for V2X communication over Uuunicast. prov_chg_uu_mbms: provisioning information change for V2X communication over Uu MBMS. prov_chg_uu_pc5: provisioning information change for V2X communication over PC5. v2x_msg: V2X interoperability message. pred_qos: information on the predicted QoS | 

### Return type

[**SubscriptionLinkList**](SubscriptionLinkList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SubPOST**
> SubscriptionsBody SubPOST(ctx, body)
 create a new subscription to VIS notifications.

 create a new subscription to VIS notifications.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsBody**](SubscriptionsBody.md)|  | 

### Return type

[**SubscriptionsBody**](subscriptions_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

