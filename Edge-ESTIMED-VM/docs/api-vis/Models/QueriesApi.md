# {{classname}}

All URIs are relative to *https://localhost/vis/v2*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ProvInfoGET**](QueriesApi.md#ProvInfoGET) | **Get** /queries/pc5_provisioning_info | Query provisioning information for V2X communication over PC5.
[**ProvInfoUuMbmsGET**](QueriesApi.md#ProvInfoUuMbmsGET) | **Get** /queries/uu_mbms_provisioning_info | retrieve information required for V2X communication over Uu MBMS.
[**ProvInfoUuUnicastGET**](QueriesApi.md#ProvInfoUuUnicastGET) | **Get** /queries/uu_unicast_provisioning_info | Used to query provisioning information for V2X communication over Uu unicast.

# **ProvInfoGET**
> Pc5ProvisioningInfo ProvInfoGET(ctx, locationInfo)
Query provisioning information for V2X communication over PC5.

Query provisioning information for V2X communication over PC5.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **locationInfo** | **string**| Comma separated list of locations to identify a cell of a base station or a particular geographical area | 

### Return type

[**Pc5ProvisioningInfo**](Pc5ProvisioningInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ProvInfoUuMbmsGET**
> UuMbmsProvisioningInfo ProvInfoUuMbmsGET(ctx, locationInfo)
retrieve information required for V2X communication over Uu MBMS.

retrieve information required for V2X communication over Uu MBMS.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **locationInfo** | **string**| omma separated list of locations to identify a cell of a base station or a particular geographical area | 

### Return type

[**UuMbmsProvisioningInfo**](UuMbmsProvisioningInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ProvInfoUuUnicastGET**
> UuUnicastProvisioningInfo ProvInfoUuUnicastGET(ctx, locationInfo)
Used to query provisioning information for V2X communication over Uu unicast.

Used to query provisioning information for V2X communication over Uu unicast.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **locationInfo** | **string**| Comma separated list of locations to identify a cell of a base station or a particular geographical area | 

### Return type

[**UuUnicastProvisioningInfo**](UuUnicastProvisioningInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

