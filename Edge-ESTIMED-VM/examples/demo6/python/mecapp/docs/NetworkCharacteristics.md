# NetworkCharacteristics

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**latency** | **int** | Latency in ms | [optional] 
**latency_variation** | **int** | Latency variation in ms | [optional] 
**latency_distribution** | **str** | Latency distribution. Can only be set in the Scenario Deployment network characteristics, ignored otherwise. Latency distribution is set for the whole network and applied to every end-to-end traffic flows. Default value is &#x27;Normal&#x27; distribution. | [optional] 
**throughput** | **int** | **DEPRECATED** As of release 1.5.0, replaced by throughputUl and throughputDl | [optional] 
**throughput_dl** | **int** | Downlink throughput limit in Mbps | [optional] 
**throughput_ul** | **int** | Uplink throughput limit in Mbps | [optional] 
**packet_loss** | **float** | Packet loss percentage | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

