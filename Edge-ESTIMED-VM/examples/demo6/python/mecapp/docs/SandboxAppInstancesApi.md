# swagger_client.SandboxAppInstancesApi

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**sandbox_app_instances_delete**](SandboxAppInstancesApi.md#sandbox_app_instances_delete) | **DELETE** /sandboxAppInstances/{sandbox_name}/{app_instance_id} | Delete an existing application instance
[**sandbox_app_instances_get**](SandboxAppInstancesApi.md#sandbox_app_instances_get) | **GET** /sandboxAppInstances/{sandbox_name} | Get the list of the available application instance identifiers
[**sandbox_app_instances_post**](SandboxAppInstancesApi.md#sandbox_app_instances_post) | **POST** /sandboxAppInstances/{sandbox_name} | Create a new application instance identifier

# **sandbox_app_instances_delete**
> sandbox_app_instances_delete(sandbox_name, app_instance_id)

Delete an existing application instance

This method removes an existing application instance

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxAppInstancesApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier
app_instance_id = 'app_instance_id_example' # str | It uniquely identifies a MEC application instance identifier

try:
    # Delete an existing application instance
    api_instance.sandbox_app_instances_delete(sandbox_name, app_instance_id)
except ApiException as e:
    print("Exception when calling SandboxAppInstancesApi->sandbox_app_instances_delete: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 
 **app_instance_id** | **str**| It uniquely identifies a MEC application instance identifier | 

### Return type

void (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sandbox_app_instances_get**
> list[ApplicationInfo] sandbox_app_instances_get(sandbox_name)

Get the list of the available application instance identifiers

This method retrieves the list of the available application instance identifiers

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxAppInstancesApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier

try:
    # Get the list of the available application instance identifiers
    api_response = api_instance.sandbox_app_instances_get(sandbox_name)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling SandboxAppInstancesApi->sandbox_app_instances_get: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 

### Return type

[**list[ApplicationInfo]**](ApplicationInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sandbox_app_instances_post**
> list[ApplicationInfo] sandbox_app_instances_post(body, sandbox_name)

Create a new application instance identifier

This method creates a new application instance

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxAppInstancesApi()
body = swagger_client.ApplicationInfo() # ApplicationInfo | Pet to add to the store
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier

try:
    # Create a new application instance identifier
    api_response = api_instance.sandbox_app_instances_post(body, sandbox_name)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling SandboxAppInstancesApi->sandbox_app_instances_post: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **body** | [**ApplicationInfo**](ApplicationInfo.md)| Pet to add to the store | 
 **sandbox_name** | **str**| Sandbox identifier | 

### Return type

[**list[ApplicationInfo]**](ApplicationInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

