# swagger_client.SandboxMECServicesApi

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**sandbox_mec_services_get**](SandboxMECServicesApi.md#sandbox_mec_services_get) | **GET** /sandboxMecServices/{sandbox_name} | Get the list of the available MEC services

# **sandbox_mec_services_get**
> list[SandboxMecServices] sandbox_mec_services_get(sandbox_name)

Get the list of the available MEC services

This method retrieves the list of the available MEC services.

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxMECServicesApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier

try:
    # Get the list of the available MEC services
    api_response = api_instance.sandbox_mec_services_get(sandbox_name)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling SandboxMECServicesApi->sandbox_mec_services_get: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 

### Return type

[**list[SandboxMecServices]**](SandboxMecServices.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

