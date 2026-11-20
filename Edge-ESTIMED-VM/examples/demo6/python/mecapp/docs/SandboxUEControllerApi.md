# swagger_client.SandboxUEControllerApi

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**sandbox_ue_controller_get**](SandboxUEControllerApi.md#sandbox_ue_controller_get) | **GET** /sandboxUeController/{sandbox_name} | Get the list of the available UEs (e.g. \&quot;Stationary UE\&quot;)
[**sandbox_ue_controller_patch**](SandboxUEControllerApi.md#sandbox_ue_controller_patch) | **PATCH** /sandboxUeController/{sandbox_name} | set the new value of the UE

# **sandbox_ue_controller_get**
> list[UE] sandbox_ue_controller_get(sandbox_name)

Get the list of the available UEs (e.g. \"Stationary UE\")

This method retrieves the list of the available available UEs.

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxUEControllerApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier

try:
    # Get the list of the available UEs (e.g. \"Stationary UE\")
    api_response = api_instance.sandbox_ue_controller_get(sandbox_name)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling SandboxUEControllerApi->sandbox_ue_controller_get: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 

### Return type

[**list[UE]**](UE.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sandbox_ue_controller_patch**
> sandbox_ue_controller_patch(sandbox_name, user_equipment_id, user_equipment_value)

set the new value of the UE

This method sets the new value of the UE.

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxUEControllerApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier
user_equipment_id = 'user_equipment_id_example' # str | User equipmenet identifier
user_equipment_value = 56 # int | It uniquely identifies a UE to set the new value

try:
    # set the new value of the UE
    api_instance.sandbox_ue_controller_patch(sandbox_name, user_equipment_id, user_equipment_value)
except ApiException as e:
    print("Exception when calling SandboxUEControllerApi->sandbox_ue_controller_patch: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 
 **user_equipment_id** | **str**| User equipmenet identifier | 
 **user_equipment_value** | **int**| It uniquely identifies a UE to set the new value | 

### Return type

void (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

