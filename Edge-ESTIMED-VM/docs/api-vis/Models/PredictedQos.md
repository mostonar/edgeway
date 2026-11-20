# PredictedQos

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**LocationGranularity** | **string** | Granularity of visited location. Measured in meters. | [default to null]
**NoticePeriod** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**PredictionArea** | [***PredictionArea**](PredictionArea.md) |  | [optional] [default to null]
**PredictionTarget** | **string** | Indicates target of QoS prediction. Valid values:  1. SINGLE_UE_PREDICTION:  The predicted QoS is to be intended as journey-specific for a requesting vehicular UE.  2. E2E_APPLICATION_INSTANCE_PREDICTION: The E2E user plane link between two V2X application instances, where one instance relates to a single vehicular UE and the other instance to an application instance within another network, i.e. either another vehicular UE as in the V2N2V case, or an infrastructure element as in the V2N2I case.  Shall only be included in the request.   | [default to null]
**Qos** | [***Qos**](Qos.md) |  | [default to null]
**Routes** | [**[]Routes**](Routes.md) |  | [optional] [default to null]
**TimeGranularity** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

