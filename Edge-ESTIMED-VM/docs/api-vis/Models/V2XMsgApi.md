# {{classname}}

All URIs are relative to *https://localhost/vis/v2*

Method | HTTP request | Description
------------- | ------------- | -------------
[**V2xMessagePOST**](V2XMsgApi.md#V2xMessagePOST) | **Post** /publish_v2x_message | Used to publish a V2X message.
[**V2xMsgDistributionServerPost**](V2XMsgApi.md#V2xMsgDistributionServerPost) | **Post** /provide_v2x_msg_distribution_server_info | Request the information about available V2X Message Distribution Servers that can be supported by the service consumer.

# **V2xMessagePOST**
> V2xMessagePOST(ctx, body)
Used to publish a V2X message.

Used to publish a V2X message.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**V2xMsgPublication**](V2xMsgPublication.md)|  | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **V2xMsgDistributionServerPost**
> V2xMsgDistributionServerInfo V2xMsgDistributionServerPost(ctx, body)
Request the information about available V2X Message Distribution Servers that can be supported by the service consumer.

Request the information about available V2X Message Distribution Servers that can be supported by the service consumer (e.g. a MEC application).

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**V2xMsgDistributionServerInfo**](V2xMsgDistributionServerInfo.md)|  | 

### Return type

[**V2xMsgDistributionServerInfo**](V2xMsgDistributionServerInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

