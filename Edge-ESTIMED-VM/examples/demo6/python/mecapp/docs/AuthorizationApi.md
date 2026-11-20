# swagger_client.AuthorizationApi

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**login**](AuthorizationApi.md#login) | **POST** /login | Initiate OAuth login procedure and creates a MEC Sandbox instance
[**logout**](AuthorizationApi.md#logout) | **POST** /logout | Terminates User Session and delete the Sandbox instance

# **login**
> Sandbox login(provider)

Initiate OAuth login procedure and creates a MEC Sandbox instance

Initiate OAuth login procedure and creates a MEC Sandbox instance

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.AuthorizationApi()
provider = 'provider_example' # str | Oauth provider

try:
    # Initiate OAuth login procedure and creates a MEC Sandbox instance
    api_response = api_instance.login(provider)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling AuthorizationApi->login: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **provider** | **str**| Oauth provider | 

### Return type

[**Sandbox**](Sandbox.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **logout**
> logout(sandbox_name)

Terminates User Session and delete the Sandbox instance

Terminates User Session and delete the Sandbox instance

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.AuthorizationApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier

try:
    # Terminates User Session and delete the Sandbox instance
    api_instance.logout(sandbox_name)
except ApiException as e:
    print("Exception when calling AuthorizationApi->logout: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 

### Return type

void (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

