# swagger_client.SandboxLogsSubscriptionsApi

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**sandbox_logs_subscriptions_delete**](SandboxLogsSubscriptionsApi.md#sandbox_logs_subscriptions_delete) | **DELETE** /sandboxLogsSubscriptions/{sandbox_name}/{subscription_reference} | Subscription to receive logs from the sandbox
[**sandbox_logs_subscriptions_post**](SandboxLogsSubscriptionsApi.md#sandbox_logs_subscriptions_post) | **POST** /sandboxLogsSubscriptions/{sandbox_name} | Subscription to receive logs from the sandbox

# **sandbox_logs_subscriptions_delete**
> sandbox_logs_subscriptions_delete(sandbox_name, subscription_reference)

Subscription to receive logs from the sandbox

This method is used to receive logs from the sandbox.

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxLogsSubscriptionsApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier
subscription_reference = 'subscription_reference_example' # str | It uniquely identifies subscription reference to receive logs from the sandbox

try:
    # Subscription to receive logs from the sandbox
    api_instance.sandbox_logs_subscriptions_delete(sandbox_name, subscription_reference)
except ApiException as e:
    print("Exception when calling SandboxLogsSubscriptionsApi->sandbox_logs_subscriptions_delete: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 
 **subscription_reference** | **str**| It uniquely identifies subscription reference to receive logs from the sandbox | 

### Return type

void (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sandbox_logs_subscriptions_post**
> list[SandboxLogsSubscriptions] sandbox_logs_subscriptions_post(sandbox_name)

Subscription to receive logs from the sandbox

This method is used to receive logs from the sandbox.

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxLogsSubscriptionsApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier

try:
    # Subscription to receive logs from the sandbox
    api_response = api_instance.sandbox_logs_subscriptions_post(sandbox_name)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling SandboxLogsSubscriptionsApi->sandbox_logs_subscriptions_post: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 

### Return type

[**list[SandboxLogsSubscriptions]**](SandboxLogsSubscriptions.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

