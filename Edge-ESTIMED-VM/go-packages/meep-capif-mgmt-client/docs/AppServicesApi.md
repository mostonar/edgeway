# {{classname}}

All URIs are relative to *https://localhost/sandboxname/service-apis/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**AppServicesGET**](AppServicesApi.md#AppServicesGET) | **Get** /{apfId}/service-apis | get services
[**AppServicesPOST**](AppServicesApi.md#AppServicesPOST) | **Post** /{apfId}/service-apis | Publish a new API
[**AppServicesServiceIdDELETE**](AppServicesApi.md#AppServicesServiceIdDELETE) | **Delete** /{apfId}/service-apis/{serviceApiId} | 
[**AppServicesServiceIdGET**](AppServicesApi.md#AppServicesServiceIdGET) | **Get** /{apfId}/service-apis/{serviceApiId} | 
[**AppServicesServiceIdPATCH**](AppServicesApi.md#AppServicesServiceIdPATCH) | **Patch** /{apfId}/service-apis/{serviceApiId} | 
[**AppServicesServiceIdPUT**](AppServicesApi.md#AppServicesServiceIdPUT) | **Put** /{apfId}/service-apis/{serviceApiId} | 

# **AppServicesGET**
> []ServiceApiDescription AppServicesGET(ctx, apfId)
get services

This method retrieves information about a list of mecService resources. This method is typically used in \"service availability query\" procedure

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **apfId** | **string**| Identifier of the entity that registers the service API. If the MEC app plays the role of the APF, this variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6). If the MEC platform plays the role of the APF, this variable shall be set to a specific identifier that identifies the MEC platform.  | 

### Return type

[**[]ServiceApiDescription**](ServiceAPIDescription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AppServicesPOST**
> ServiceApiDescription AppServicesPOST(ctx, body, apfId)
Publish a new API

Enables a service consumer to request to publish a new API at the CCF.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**ServiceApiDescription**](ServiceApiDescription.md)|  | 
  **apfId** | **string**| Identifier of the entity that registers the service API. If the MEC app plays the role of the APF, this variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6). If the MEC platform plays the role of the APF, this variable shall be set to a specific identifier that identifies the MEC platform.  | 

### Return type

[**ServiceApiDescription**](ServiceAPIDescription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AppServicesServiceIdDELETE**
> AppServicesServiceIdDELETE(ctx, apfId, serviceApiId)


The HTTP DELETE method allows a service consumer to delete an existing \"Individual APF published API\" resource  at the CCF

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **apfId** | **string**| Identifier of the entity that registers the service API. If the MEC app plays the role of the APF, this variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6). If the MEC platform plays the role of the APF, this variable shall be set to a specific identifier that identifies the MEC platform.  | 
  **serviceApiId** | **string**| Represents a MEC service instance. | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AppServicesServiceIdGET**
> []ServiceApiDescription AppServicesServiceIdGET(ctx, apfId, serviceApiId)


The HTTP GET method allows a service consumer to retrieve an existing \"Individual APF published API\" resource at  the CCF.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **apfId** | **string**| Identifier of the entity that registers the service API. If the MEC app plays the role of the APF, this variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6). If the MEC platform plays the role of the APF, this variable shall be set to a specific identifier that identifies the MEC platform.  | 
  **serviceApiId** | **string**| Represents a MEC service instance. | 

### Return type

[**[]ServiceApiDescription**](ServiceAPIDescription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AppServicesServiceIdPATCH**
> []ServiceApiDescription AppServicesServiceIdPATCH(ctx, body, apfId, serviceApiId)


The HTTP PATCH method allows a service consumer to modify an existing \"Individual APF published API\" resource at the CCF.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**ServiceApiDescriptionPatch**](ServiceApiDescriptionPatch.md)|  | 
  **apfId** | **string**| Identifier of the entity that registers the service API. If the MEC app plays the role of the APF, this variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6). If the MEC platform plays the role of the APF, this variable shall be set to a specific identifier that identifies the MEC platform.  | 
  **serviceApiId** | **string**| Represents a MEC service instance. | 

### Return type

[**[]ServiceApiDescription**](ServiceAPIDescription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **AppServicesServiceIdPUT**
> []ServiceApiDescription AppServicesServiceIdPUT(ctx, body, apfId, serviceApiId)


The HTTP PUT method allows a service consumer to update an existing \"Individual APF published API\" resource at the CCF.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**ServiceApiDescription**](ServiceApiDescription.md)|  | 
  **apfId** | **string**| Identifier of the entity that registers the service API. If the MEC app plays the role of the APF, this variable shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6). If the MEC platform plays the role of the APF, this variable shall be set to a specific identifier that identifies the MEC platform.  | 
  **serviceApiId** | **string**| Represents a MEC service instance. | 

### Return type

[**[]ServiceApiDescription**](ServiceAPIDescription.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

