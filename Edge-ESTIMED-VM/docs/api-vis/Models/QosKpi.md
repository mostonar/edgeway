# QosKpi

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Confidence** | **string** | Confidence of the prediction, as returned by the relevant domain PF. The value and the measurement of the confidence depends on the SLA. Shall only be included in the response. | [optional] [default to null]
**KpiName** | **string** | The name of the KPI (e.g. latency, UL bitrate, etc). It can be included in the request and in the response. | [default to null]
**KpiValue** | **string** | Information on the predicted value for the specific QoS KPI. It can be in different forms, such as upper bound and lower bound, CDF, actual value, etc. Shall only be included in the response. | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

