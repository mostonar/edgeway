# {{classname}}

All URIs are relative to *http://localhost/sandbox-api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxIndividualNetworkScenariosGET**](SandboxNetworkScenariosApi.md#SandboxIndividualNetworkScenariosGET) | **Get** /sandboxNetworkScenarios/{sandbox_name} | Get description of a Network Scenario to be used.
[**SandboxNetworkScenarioDELETE**](SandboxNetworkScenariosApi.md#SandboxNetworkScenarioDELETE) | **Delete** /sandboxNetworkScenarios/{sandbox_name}/{network_scenario_id} | Deactivate the Network Scenario.
[**SandboxNetworkScenarioPOST**](SandboxNetworkScenariosApi.md#SandboxNetworkScenarioPOST) | **Post** /sandboxNetworkScenarios/{sandbox_name} | Selects the Network Scenario to be activated.
[**SandboxNetworkScenariosGET**](SandboxNetworkScenariosApi.md#SandboxNetworkScenariosGET) | **Get** /sandboxNetworkScenarios | Get the list of the available network scenarios

# **SandboxIndividualNetworkScenariosGET**
> []Scenario SandboxIndividualNetworkScenariosGET(ctx, sandboxName, networkScenarioId)
Get description of a Network Scenario to be used.

This method retrive description of a the network scenario

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 
  **networkScenarioId** | **string**| Network scenario to retrieve | 

### Return type

[**[]Scenario**](Scenario.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxNetworkScenarioDELETE**
> SandboxNetworkScenarioDELETE(ctx, sandboxName, networkScenarioId)
Deactivate the Network Scenario.

This method deactivates the network scenario

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 
  **networkScenarioId** | **string**| Network scenario to be used | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxNetworkScenarioPOST**
> SandboxNetworkScenarioPOST(ctx, sandboxName, networkScenarioId)
Selects the Network Scenario to be activated.

This method selects the network scenario to be activated. This request initiates the creation of necessary MEC services for specific network scenario

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 
  **networkScenarioId** | **string**| Network scenario to be used | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxNetworkScenariosGET**
> []SandboxNetworkScenario SandboxNetworkScenariosGET(ctx, sandboxName)
Get the list of the available network scenarios

This method retrieves the list of the available network scenarios.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **sandboxName** | **string**| Sandbox identifier | 

### Return type

[**[]SandboxNetworkScenario**](SandboxNetworkScenario.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

