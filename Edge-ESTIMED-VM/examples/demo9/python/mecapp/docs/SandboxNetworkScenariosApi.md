# swagger_client.SandboxNetworkScenariosApi

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**sandbox_individual_network_scenarios_get**](SandboxNetworkScenariosApi.md#sandbox_individual_network_scenarios_get) | **GET** /sandboxNetworkScenarios/{sandbox_name} | Get description of a Network Scenario to be used.
[**sandbox_network_scenario_delete**](SandboxNetworkScenariosApi.md#sandbox_network_scenario_delete) | **DELETE** /sandboxNetworkScenarios/{sandbox_name}/{network_scenario_id} | Deactivate the Network Scenario.
[**sandbox_network_scenario_post**](SandboxNetworkScenariosApi.md#sandbox_network_scenario_post) | **POST** /sandboxNetworkScenarios/{sandbox_name} | Selects the Network Scenario to be activated.
[**sandbox_network_scenarios_get**](SandboxNetworkScenariosApi.md#sandbox_network_scenarios_get) | **GET** /sandboxNetworkScenarios | Get the list of the available network scenarios

# **sandbox_individual_network_scenarios_get**
> list[Scenario] sandbox_individual_network_scenarios_get(sandbox_name, network_scenario_id)

Get description of a Network Scenario to be used.

This method retrive description of a the network scenario

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxNetworkScenariosApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier
network_scenario_id = 'network_scenario_id_example' # str | Network scenario to retrieve

try:
    # Get description of a Network Scenario to be used.
    api_response = api_instance.sandbox_individual_network_scenarios_get(sandbox_name, network_scenario_id)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling SandboxNetworkScenariosApi->sandbox_individual_network_scenarios_get: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 
 **network_scenario_id** | **str**| Network scenario to retrieve | 

### Return type

[**list[Scenario]**](Scenario.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sandbox_network_scenario_delete**
> sandbox_network_scenario_delete(sandbox_name, network_scenario_id)

Deactivate the Network Scenario.

This method deactivates the network scenario

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxNetworkScenariosApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier
network_scenario_id = 'network_scenario_id_example' # str | Network scenario to be used

try:
    # Deactivate the Network Scenario.
    api_instance.sandbox_network_scenario_delete(sandbox_name, network_scenario_id)
except ApiException as e:
    print("Exception when calling SandboxNetworkScenariosApi->sandbox_network_scenario_delete: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 
 **network_scenario_id** | **str**| Network scenario to be used | 

### Return type

void (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sandbox_network_scenario_post**
> sandbox_network_scenario_post(sandbox_name, network_scenario_id)

Selects the Network Scenario to be activated.

This method selects the network scenario to be activated. This request initiates the creation of necessary MEC services for specific network scenario

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxNetworkScenariosApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier
network_scenario_id = 'network_scenario_id_example' # str | Network scenario to be used

try:
    # Selects the Network Scenario to be activated.
    api_instance.sandbox_network_scenario_post(sandbox_name, network_scenario_id)
except ApiException as e:
    print("Exception when calling SandboxNetworkScenariosApi->sandbox_network_scenario_post: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 
 **network_scenario_id** | **str**| Network scenario to be used | 

### Return type

void (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sandbox_network_scenarios_get**
> list[SandboxNetworkScenario] sandbox_network_scenarios_get(sandbox_name)

Get the list of the available network scenarios

This method retrieves the list of the available network scenarios.

### Example
```python
from __future__ import print_function
import time
import swagger_client
from swagger_client.rest import ApiException
from pprint import pprint

# create an instance of the API class
api_instance = swagger_client.SandboxNetworkScenariosApi()
sandbox_name = 'sandbox_name_example' # str | Sandbox identifier

try:
    # Get the list of the available network scenarios
    api_response = api_instance.sandbox_network_scenarios_get(sandbox_name)
    pprint(api_response)
except ApiException as e:
    print("Exception when calling SandboxNetworkScenariosApi->sandbox_network_scenarios_get: %s\n" % e)
```

### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **sandbox_name** | **str**| Sandbox identifier | 

### Return type

[**list[SandboxNetworkScenario]**](SandboxNetworkScenario.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

