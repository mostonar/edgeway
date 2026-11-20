# {{classname}}

All URIs are relative to *http://127.0.0.1:8081/mec_service_mgmt/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**GetIndividualMECService**](IndividualMECserviceApi.md#GetIndividualMECService) | **Get** /resource_uri_allocated_by_MEC_platform | get mecServiceLiveness
[**PatchIndividualMECService**](IndividualMECserviceApi.md#PatchIndividualMECService) | **Patch** /resource_uri_allocated_by_MEC_platform | Update mecServiceLiveness

# **GetIndividualMECService**
> ServiceLivenessInfo GetIndividualMECService(ctx, )
get mecServiceLiveness

This method retrieves information about an \"Individual mecServiceLiveness\" resource

### Required Parameters
This endpoint does not need any parameter.

### Return type

[**ServiceLivenessInfo**](ServiceLivenessInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **PatchIndividualMECService**
> ServiceLivenessInfo PatchIndividualMECService(ctx, body)
Update mecServiceLiveness

This method updates a resource on top of the existing resource state with partial changes described by the client.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**ServiceLivenessUpdate**](ServiceLivenessUpdate.md)| It contains an update of the liveness state. | 

### Return type

[**ServiceLivenessInfo**](ServiceLivenessInfo.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

