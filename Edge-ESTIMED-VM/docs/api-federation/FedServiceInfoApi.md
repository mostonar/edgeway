# {{classname}}

All URIs are relative to *https://localhost/sandboxname/fed_enablement/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ServiceGET**](FedServiceInfoApi.md#ServiceGET) | **Get** /fed_resources/systems/{systemId}/services/{serviceId} | Retrieves the information of a specific MEC service hosted by the MEC system associated with the systemId and serviceId
[**ServicesGET**](FedServiceInfoApi.md#ServicesGET) | **Get** /fed_resources/systems/{systemId}/services | Retrieves the information of all MEC services hosted by the MEC system

# **ServiceGET**
> FedServiceInfo ServiceGET(ctx, systemId, serviceId)
Retrieves the information of a specific MEC service hosted by the MEC system associated with the systemId and serviceId

Retrieves the information of a specific MEC service hosted by the MEC system associated with the systemId and serviceId

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **systemId** | **string**| Identifier of the MEC system. | 
  **serviceId** | **string**| Service identifier. | 

### Return type

[**FedServiceInfo**](FedServiceInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ServicesGET**
> []FedServiceInfo ServicesGET(ctx, systemId, optional)
Retrieves the information of all MEC services hosted by the MEC system

Retrieves the information of all MEC services hosted by the MEC system associated with the systemId

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **systemId** | **string**| Identifier of the MEC system. | 
 **optional** | ***FedServiceInfoApiServicesGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a FedServiceInfoApiServicesGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **seInstancerId** | **optional.String**| Indicator of a MEC service offered for MEC federation. | 
 **serName** | **optional.String**| The name of the service. | 
 **serCategory** | **optional.String**| A Category reference. | 

### Return type

[**[]FedServiceInfo**](FedServiceInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

