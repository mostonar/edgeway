# Domain

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique domain ID | [optional] 
**name** | **str** | Domain name | [optional] 
**type** | **str** | Domain type | [optional] 
**net_char** | [**NetworkCharacteristics**](NetworkCharacteristics.md) |  | [optional] 
**inter_zone_latency** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latency | [optional] 
**inter_zone_latency_variation** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latencyVariation | [optional] 
**inter_zone_throughput** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar throughputUl and throughputDl | [optional] 
**inter_zone_packet_loss** | **float** | **DEPRECATED** As of release 1.5.0, replaced by netChar packetLoss | [optional] 
**meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**user_meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**cellular_domain_config** | [**CellularDomainConfig**](CellularDomainConfig.md) |  | [optional] 
**zones** | [**list[Zone]**](Zone.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

