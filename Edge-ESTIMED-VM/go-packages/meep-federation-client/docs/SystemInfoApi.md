# {{classname}}

All URIs are relative to *https://localhost/sandboxname/fed_enablement/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SysteminfoByIdDELETE**](SystemInfoApi.md#SysteminfoByIdDELETE) | **Delete** /fed_resources/systems/{systemId} | Delete the systems resource for a given MEC system.
[**SysteminfoByIdGET**](SystemInfoApi.md#SysteminfoByIdGET) | **Get** /fed_resources/systems/{systemId} | Retrieve the systems resource of the federation member with systemId as its system identifier.
[**SysteminfoByIdPATCH**](SystemInfoApi.md#SysteminfoByIdPATCH) | **Patch** /fed_resources/systems/{systemId} | Update the systems resource for a given MEC system.
[**SysteminfoGET**](SystemInfoApi.md#SysteminfoGET) | **Get** /fed_resources/systems | Retrieve a list of systems resources (see clause 6 for data model) of federation members.
[**SysteminfoPOST**](SystemInfoApi.md#SysteminfoPOST) | **Post** /fed_resources/systems | Create new systems resource for a given MEC system.

# **SysteminfoByIdDELETE**
> SysteminfoByIdDELETE(ctx, systemId)
Delete the systems resource for a given MEC system.

The DELETE method deletes the information of systems resources stored in the MEF. This method is typically used in the sequence of \"Deregistration of MEC system to the federation\" as described in clause 5.2.2.1.1. The method shall comply with the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.4.3.5-1 and 7.4.3.5-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **systemId** | **string**| Identifier of the MEC system. | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SysteminfoByIdGET**
> FedResourcesSystemsBody SysteminfoByIdGET(ctx, systemId)
Retrieve the systems resource of the federation member with systemId as its system identifier.

The GET method retrieves the systems resource information. This method is typically used in the sequence of \"MEC system discovery\" as described in clause 5.2.2.2. The method shall comply with the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.4.3.1-1 and 7.4.3.1-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **systemId** | **string**| Identifier of the MEC system. | 

### Return type

[**FedResourcesSystemsBody**](fed_resources_systems_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SysteminfoByIdPATCH**
> FedResourcesSystemsBody SysteminfoByIdPATCH(ctx, body, systemId)
Update the systems resource for a given MEC system.

The PATCH method updates the information of systems resources stored in the MEC federator through previous registration. This method is typically used in the sequence of \"Update of MEC system to the federation\" as described in clause 5.2.2.2. The method shall comply with the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.4.3.3-1 and 7.4.3.3-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**SystemsSystemIdBody**](SystemsSystemIdBody.md)| It contains attributes to be update. | 
  **systemId** | **string**| Identifier of the MEC system. | 

### Return type

[**FedResourcesSystemsBody**](fed_resources_systems_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SysteminfoGET**
> []SystemInfo SysteminfoGET(ctx, optional)
Retrieve a list of systems resources (see clause 6 for data model) of federation members.

The GET method retrieves the information of a list of systems resources of federation members. This method is typically used in the sequence of \"MEC system discovery\" as described in clause 5.2.2.2. The method shall comply with the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.3.3.1-1 and 7.3.3.1-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***SystemInfoApiSysteminfoGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a SystemInfoApiSysteminfoGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **systemId** | [**optional.Interface of []string**](string.md)| Identifier of the MEC system. | 
 **systemName** | [**optional.Interface of []string**](string.md)| The name of the MEC system. | 
 **systemProvider** | [**optional.Interface of []string**](string.md)| Provider of the MEC system. | 

### Return type

[**[]SystemInfo**](SystemInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **SysteminfoPOST**
> FedResourcesSystemsBody SysteminfoPOST(ctx, body)
Create new systems resource for a given MEC system.

The POST method creates the information of systems resources to the MEC federator. This method is typically used in the sequence of \"Registration of MEC system to the federation\" as described in clause 5.2.2.1. The method shall comply with the URI query parameters, request and response data structures, and response codes, as specified in Tables 7.3.3.4-1 and 7.3.3.4-2.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**FedResourcesSystemsBody**](FedResourcesSystemsBody.md)| Entity body in the request contains SystemInfo to be created. The attribute &quot;systemId&quot; shall be absent. | 

### Return type

[**FedResourcesSystemsBody**](fed_resources_systems_body.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

