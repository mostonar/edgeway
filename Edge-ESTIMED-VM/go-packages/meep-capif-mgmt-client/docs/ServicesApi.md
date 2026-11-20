# {{classname}}

All URIs are relative to *https://localhost/sandboxname/service-apis/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ServicesGET**](ServicesApi.md#ServicesGET) | **Get** /allServiceAPIs | 

# **ServicesGET**
> []DiscoveredApis ServicesGET(ctx, optional)


This method retrieves information about a list of mecService resources. This method is typically used in \"service availability query\" procedure

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
 **optional** | ***ServicesApiServicesGETOpts** | optional parameters | nil if no parameters

### Optional Parameters
Optional parameters are passed through a pointer to a ServicesApiServicesGETOpts struct
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **vendSpecEtsiMecSerinstanceId** | **optional.String**| Vendor-specific query parameter for service instance ID. The \&quot;target\&quot; attribute shall be set to \&quot;/apiId\&quot;. | 
 **apiName** | [**optional.Interface of []string**](string.md)| This query parameter corresponds to the parameter \&quot;ser_name\&quot; as defined in clause 8.2.3.3.1. As opposed to the cardinality of ser_instance_id defined in clause 8.2.3.3.1 that is 0..N, this parameter only supports a cardinality of 0..1. | 
 **apiInvokerId** | **optional.String**| If the Discover_Service_API is produced by the MEC platform towards the MEC applications, this identifier shall be set to the value of the \&quot;appInstanceId\&quot; (see clause 7.1.2.6). If the Discover_Service_API is produced by the CCF, the provisions in clause 8.1.2.2.3.1 of 3GPP TS 29.222 [21] apply.  | 
 **vendSpecEtsiMecSercategoryId** | **optional.String**| Vendor-specific query parameter for service category ID. The \&quot;target\&quot; attribute shall be set to \&quot;/vendorSpecific-urn:etsi:mec:capifext:service-info/serCategory/id\&quot;. | 
 **vendSpecEtsiMecConsumedLocalOnly** | **optional.String**| Vendor-specific query parameter for consumed local only. The \&quot;target\&quot; attribute shall be set to \&quot;/vendorSpecific-urn:etsi:mec:capifext:service-info/consumedLocalOnly\&quot;. | 
 **vendSpecEtsiMecIsLocal** | **optional.String**| Vendor-specific query parameter for is local. The \&quot;target\&quot; attribute shall be set to \&quot;/vendorSpecific-urn:etsi:mec:capifext:service-info/isLocal\&quot;. | 
 **vendSpecEtsiMecScopeOfLocality** | **optional.String**| Vendor-specific query parameter for scope of locality. The \&quot;target\&quot; attribute shall be set to \&quot;/vendorSpecific-urn:etsi:mec:capifext:service-info/scopeOfLocality\&quot;. | 

### Return type

[**[]DiscoveredApis**](DiscoveredAPIs.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

