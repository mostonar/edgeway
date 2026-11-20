# {{classname}}

All URIs are relative to *https://localhost/vis/v2*

Method | HTTP request | Description
------------- | ------------- | -------------
[**PredictedQosPOST**](QoSApi.md#PredictedQosPOST) | **Post** /provide_predicted_qos | Request the predicted QoS correspondent to potential routes of a vehicular UE.

# **PredictedQosPOST**
> PredictedQos PredictedQosPOST(ctx, body)
Request the predicted QoS correspondent to potential routes of a vehicular UE.

Request the predicted QoS correspondent to potential routes of a vehicular UE.

### Required Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
  **body** | [**PredictedQos**](PredictedQos.md)|  | 

### Return type

[**PredictedQos**](PredictedQos.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

