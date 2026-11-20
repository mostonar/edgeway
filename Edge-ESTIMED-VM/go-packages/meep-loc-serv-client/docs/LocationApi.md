# {{classname}}

All URIs are relative to *https://localhost/sandboxname/location/v3*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ApByIdGET**](LocationApi.md#ApByIdGET) | **Get** /queries/zones/{zoneId}/accessPoints/{accessPointId} | Radio Node Location Lookup
[**ApGET**](LocationApi.md#ApGET) | **Get** /queries/zones/{zoneId}/accessPoints | Radio Node Location Lookup
[**AreaSubDELETE**](LocationApi.md#AreaSubDELETE) | **Delete** /subscriptions/area/{subscriptionId} | Cancel a subscription
[**AreaSubGET**](LocationApi.md#AreaSubGET) | **Get** /subscriptions/area/{subscriptionId} | Retrieve subscription information
[**AreaSubListGET**](LocationApi.md#AreaSubListGET) | **Get** /subscriptions/area | Retrieves information about the subscriptions for this requestor.
[**AreaSubPOST**](LocationApi.md#AreaSubPOST) | **Post** /subscriptions/area | Creates subscription to area notifications.
[**AreaSubPUT**](LocationApi.md#AreaSubPUT) | **Put** /subscriptions/area/{subscriptionId} | Updates a subscription information
[**DistanceGET**](LocationApi.md#DistanceGET) | **Get** /queries/distance | UE Distance Lookup of a specific UE
[**DistanceSubDELETE**](LocationApi.md#DistanceSubDELETE) | **Delete** /subscriptions/distance/{subscriptionId} | Cancel a subscription
[**DistanceSubGET**](LocationApi.md#DistanceSubGET) | **Get** /subscriptions/distance/{subscriptionId} | Retrieve user distance subscription information
[**DistanceSubListGET**](LocationApi.md#DistanceSubListGET) | **Get** /subscriptions/distance | Retrieves all active subscriptions to distance change notifications
[**DistanceSubPOST**](LocationApi.md#DistanceSubPOST) | **Post** /subscriptions/distance | Creates a subscription for distance change notification
[**DistanceSubPUT**](LocationApi.md#DistanceSubPUT) | **Put** /subscriptions/distance/{subscriptionId} | Updates a user distance subscription information
[**Mec011AppTerminationPOST**](LocationApi.md#Mec011AppTerminationPOST) | **Post** /notifications/mec011/appTermination | MEC011 Application Termination notification for self termination
[**UserSubDELETE**](LocationApi.md#UserSubDELETE) | **Delete** /subscriptions/users/{subscriptionId} | Cancel a subscription
[**UserSubGET**](LocationApi.md#UserSubGET) | **Get** /subscriptions/users/{subscriptionId} | Retrieve subscription information
[**UserSubListGET**](LocationApi.md#UserSubListGET) | **Get** /subscriptions/users | Retrieves information about the subscriptions for the requestor
[**UserSubPOST**](LocationApi.md#UserSubPOST) | **Post** /subscriptions/users | Create subscription to UE location notifications.
[**UserSubPUT**](LocationApi.md#UserSubPUT) | **Put** /subscriptions/users/{subscriptionId} | Updates a subscription information
[**UsersGET**](LocationApi.md#UsersGET) | **Get** /queries/users | UE Location Lookup of a specific UE or group of UEs
[**ZoneSubListGET**](LocationApi.md#ZoneSubListGET) | **Get** /subscriptions/zones | Retrieves all active subscriptions to zone notifications
[**ZoneSubPOST**](LocationApi.md#ZoneSubPOST) | **Post** /subscriptions/zones | Creates a subscription to zone notifications
[**ZonesGET**](LocationApi.md#ZonesGET) | **Get** /queries/zones | Zones information Lookup
[**ZonesGetById**](LocationApi.md#ZonesGetById) | **Get** /queries/zones/{zoneId} | Zones information Lookup

# **ApByIdGET**
> InlineAccessPointInfo ApByIdGET(ctx, zoneId, accessPointId)
Radio Node Location Lookup

Radio Node Location Lookup to retrieve a radio node associated to a zone.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **zoneId** | **string**| Indentifier of zone | 
  **accessPointId** | **string**| Identifier of access Point | 

### Return type

[**InlineAccessPointInfo**](InlineAccessPointInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ApGET**
> InlineAccessPointList ApGET(ctx, zoneId, optional)
Radio Node Location Lookup

Radio Node Location Lookup to retrieve a list of radio nodes associated to a zone.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **zoneId** | **string**| Indentifier of zone | 
 **optional** | ***LocationApiApGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a LocationApiApGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **interestRealm** | **optional.String**| Interest realm of access point (e.g. geographical area, a type of industry etc.). | 

### Return type

[**InlineAccessPointList**](InlineAccessPointList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AreaSubDELETE**
> AreaSubDELETE(ctx, subscriptionId)
Cancel a subscription

The DELETE method is used to cancel the existing subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AreaSubGET**
> InlineResponse2001 AreaSubGET(ctx, subscriptionId)
Retrieve subscription information

The GET method is used to retrieve information about this subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**InlineResponse2001**](inline_response_200_1.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AreaSubListGET**
> InlineResponse200 AreaSubListGET(ctx, optional)
Retrieves information about the subscriptions for this requestor.

The GET method is used to request information about the subscriptions for this requestor.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***LocationApiAreaSubListGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a LocationApiAreaSubListGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **subscriptionType** | **optional.String**| Filter subscriptions by type | 

### Return type

[**InlineResponse200**](inline_response_200.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AreaSubPOST**
> InlineResponse201 AreaSubPOST(ctx, body)
Creates subscription to area notifications.

The POST method is used to create a new subscription to area notifications.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsAreaBody**](SubscriptionsAreaBody.md)| Subscription to be created | 

### Return type

[**InlineResponse201**](inline_response_201.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AreaSubPUT**
> AreaSubscriptionIdBody AreaSubPUT(ctx, body, subscriptionId)
Updates a subscription information

The PUT method is used to update the existing subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**AreaSubscriptionIdBody**](AreaSubscriptionIdBody.md)| Subscription to be modified | 
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**AreaSubscriptionIdBody**](area_subscriptionId_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **DistanceGET**
> InlineTerminalDistance DistanceGET(ctx, address, optional)
UE Distance Lookup of a specific UE

UE Distance Lookup between terminals or a terminal and a location

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **address** | [**[]string**](string.md)| address of users (e.g. \&quot;sip\&quot; URI, \&quot;tel\&quot; URI, \&quot;acr\&quot; URI) | 
 **optional** | ***LocationApiDistanceGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a LocationApiDistanceGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **requester** | **optional.String**| Entity that is requesting the information | 
 **latitude** | **optional.Float32**| Latitude geo position | 
 **longitude** | **optional.Float32**| Longitude geo position | 

### Return type

[**InlineTerminalDistance**](InlineTerminalDistance.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **DistanceSubDELETE**
> DistanceSubDELETE(ctx, subscriptionId)
Cancel a subscription

The DELETE method is used to cancel the existing subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **DistanceSubGET**
> InlineUserDistanceSubscription DistanceSubGET(ctx, subscriptionId)
Retrieve user distance subscription information

The GET method is used to retrieve information about user distance subscription subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**InlineUserDistanceSubscription**](InlineUserDistanceSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **DistanceSubListGET**
> InlineNotificationSubscriptionList DistanceSubListGET(ctx, )
Retrieves all active subscriptions to distance change notifications

This operation is used for retrieving all active subscriptions to a distance change notifications.

### Required Parameters
This endpoint does not need any parameter.

### Return type

[**InlineNotificationSubscriptionList**](InlineNotificationSubscriptionList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **DistanceSubPOST**
> InlineUserDistanceSubscription DistanceSubPOST(ctx, body)
Creates a subscription for distance change notification

Creates a subscription to the Location Service for a distance change notification.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**InlineUserDistanceNotification**](InlineUserDistanceNotification.md)| Subscription to be created | 

### Return type

[**InlineUserDistanceSubscription**](InlineUserDistanceSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **DistanceSubPUT**
> InlineUserDistanceSubscription DistanceSubPUT(ctx, body, subscriptionId)
Updates a user distance subscription information

The PUT method is used to update the existing user distance subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**InlineUserDistanceSubscription**](InlineUserDistanceSubscription.md)| Subscription to be modified | 
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**InlineUserDistanceSubscription**](InlineUserDistanceSubscription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **Mec011AppTerminationPOST**
> Mec011AppTerminationPOST(ctx, body)
MEC011 Application Termination notification for self termination

Terminates itself.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**AppTerminationNotification**](AppTerminationNotification.md)| Termination notification details | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **UserSubDELETE**
> UserSubDELETE(ctx, subscriptionId)
Cancel a subscription

The DELETE method is used to cancel the existing subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **UserSubGET**
> InlineResponse2002 UserSubGET(ctx, subscriptionId)
Retrieve subscription information

The GET method is used to retrieve information about this subscription. 

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**InlineResponse2002**](inline_response_200_2.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **UserSubListGET**
> InlineNotificationSubscriptionList UserSubListGET(ctx, optional)
Retrieves information about the subscriptions for the requestor

The GET method is used to request information about the subscriptions for the requestor. 

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***LocationApiUserSubListGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a LocationApiUserSubListGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **subscriptionType** | **optional.String**| Filter subscriptions by type | 
 **address** | **optional.String**| Address of the user | 

### Return type

[**InlineNotificationSubscriptionList**](InlineNotificationSubscriptionList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **UserSubPOST**
> InlineResponse2011 UserSubPOST(ctx, body)
Create subscription to UE location notifications.

The POST method is used to create a new subscription to UE location notifications.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsUsersBody**](SubscriptionsUsersBody.md)| Subscription to be created | 

### Return type

[**InlineResponse2011**](inline_response_201_1.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **UserSubPUT**
> UsersSubscriptionIdBody UserSubPUT(ctx, body, subscriptionId)
Updates a subscription information

The PUT method is used to update the existing subscription.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**UsersSubscriptionIdBody**](UsersSubscriptionIdBody.md)| Subscription to be modified | 
  **subscriptionId** | **string**| Subscription Identifier, specifically the \&quot;self\&quot; returned in the subscription request | 

### Return type

[**UsersSubscriptionIdBody**](users_subscriptionId_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **UsersGET**
> InlineUserList UsersGET(ctx, optional)
UE Location Lookup of a specific UE or group of UEs

UE Location Lookup of a specific UE or group of UEs

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***LocationApiUsersGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a LocationApiUsersGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **zoneId** | [**optional.Interface of []string**](string.md)| Identifier of zone | 
 **accessPointId** | [**optional.Interface of []string**](string.md)| Identifier of access point | 
 **address** | [**optional.Interface of []string**](string.md)| address of users (e.g. \&quot;sip\&quot; URI, \&quot;tel\&quot; URI, \&quot;acr\&quot; URI) | 

### Return type

[**InlineUserList**](InlineUserList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ZoneSubListGET**
> InlineResponse2003 ZoneSubListGET(ctx, optional)
Retrieves all active subscriptions to zone notifications

This operation is used for retrieving all active subscriptions to zone notifications.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***LocationApiZoneSubListGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a LocationApiZoneSubListGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **subscriptionType** | **optional.String**| Query parameter to filter on a specific subscription type. Permitted values: -event -status | 
 **zoneId** | **optional.String**| The identity of the zone | 

### Return type

[**InlineResponse2003**](inline_response_200_3.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ZoneSubPOST**
> SubscriptionsZonesBody ZoneSubPOST(ctx, body)
Creates a subscription to zone notifications

The POST method is used to create a new subscription to zone notifications.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SubscriptionsZonesBody**](SubscriptionsZonesBody.md)| Subscription to be created | 

### Return type

[**SubscriptionsZonesBody**](subscriptions_zones_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ZonesGET**
> InlineZoneList ZonesGET(ctx, )
Zones information Lookup

Used to get a list of identifiers for zones authorized for use by the application.

### Required Parameters
This endpoint does not need any parameter.

### Return type

[**InlineZoneList**](InlineZoneList.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ZonesGetById**
> InlineZoneInfo ZonesGetById(ctx, zoneId)
Zones information Lookup

Used to get the information for an authorized zone for use by the application.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **zoneId** | **string**| Indentifier of zone | 

### Return type

[**InlineZoneInfo**](InlineZoneInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

