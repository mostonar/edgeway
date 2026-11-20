# {{classname}}

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxLogsSubscriptionsDELETE**](SandboxLogsSubscriptionsApi.md#SandboxLogsSubscriptionsDELETE) | **Delete** /sandboxLogsSubscriptions/{sandbox_name}/{subscription_reference} | Subscription to receive logs from the sandbox
[**SandboxLogsSubscriptionsPOST**](SandboxLogsSubscriptionsApi.md#SandboxLogsSubscriptionsPOST) | **Post** /sandboxLogsSubscriptions/{sandbox_name} | Subscription to receive logs from the sandbox

# **SandboxLogsSubscriptionsDELETE**
> SandboxLogsSubscriptionsDELETE(ctx, sandboxName, subscriptionReference)
Subscription to receive logs from the sandbox

This method is used to receive logs from the sandbox.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 
  **subscriptionReference** | **string**| It uniquely identifies subscription reference to receive logs from the sandbox | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxLogsSubscriptionsPOST**
> []SandboxLogsSubscriptions SandboxLogsSubscriptionsPOST(ctx, sandboxName)
Subscription to receive logs from the sandbox

This method is used to receive logs from the sandbox.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 

### Return type

[**[]SandboxLogsSubscriptions**](SandboxLogsSubscriptions.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

