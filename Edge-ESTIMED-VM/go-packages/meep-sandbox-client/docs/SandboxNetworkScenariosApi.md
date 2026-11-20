# {{classname}}

All URIs are relative to *http://127.0.0.1:8081/sandbox_api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SandboxNetworkScenariosGET**](SandboxNetworkScenariosApi.md#SandboxNetworkScenariosGET) | **Get** /sandboxNetworkScenarios | get the list of the available network scenarios
[**SandboxNetworkScenarioPOST**](SandboxNetworkScenariosApi.md#SandboxNetworkScenarioPOST) | **Post** /sandboxNetworkScenarios/{network_scenario_id} | Selects the Network Scenario to be used.

# **SandboxNetworkScenariosGET**
> []SandboxNetworkScenario SandboxNetworkScenariosGET(ctx, )
get the list of the available network scenarios

This method retrieves the list of the available network scenarios.

### Required Parameters
This endpoint does not need any parameter.

### Return type

[**[]SandboxNetworkScenario**](SandboxNetworkScenario.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SandboxNetworkScenarioPOST**
> SandboxNetworkScenarioPOST(ctx, networkScenarioId)
Selects the Network Scenario to be used.

This method selects the network scenario to be used. This request initiates the creation of necessary MEC services for specific network scenario

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **networkScenarioId** | **string**| Network scenario to be used | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: Not defined

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

