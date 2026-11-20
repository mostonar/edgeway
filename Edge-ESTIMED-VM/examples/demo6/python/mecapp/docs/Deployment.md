# Deployment

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**net_char** | [**NetworkCharacteristics**](NetworkCharacteristics.md) |  | [optional] 
**connectivity** | [**ConnectivityConfig**](ConnectivityConfig.md) |  | [optional] 
**d2d** | [**D2dConfig**](D2dConfig.md) |  | [optional] 
**inter_domain_latency** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latency | [optional] 
**inter_domain_latency_variation** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latencyVariation | [optional] 
**inter_domain_throughput** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar throughputUl and throughputDl | [optional] 
**inter_domain_packet_loss** | **float** | **DEPRECATED** As of release 1.5.0, replaced by netChar packetLoss | [optional] 
**meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**user_meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**domains** | [**list[Domain]**](Domain.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

