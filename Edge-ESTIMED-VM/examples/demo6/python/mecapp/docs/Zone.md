# Zone

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique zone ID | [optional] 
**name** | **str** | Zone name | [optional] 
**type** | **str** | Zone type | [optional] 
**net_char** | [**NetworkCharacteristics**](NetworkCharacteristics.md) |  | [optional] 
**inter_fog_latency** | **int** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**inter_fog_latency_variation** | **int** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**inter_fog_throughput** | **int** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**inter_fog_packet_loss** | **float** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**inter_edge_latency** | **int** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**inter_edge_latency_variation** | **int** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**inter_edge_throughput** | **int** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**inter_edge_packet_loss** | **float** | **DEPRECATED** As of release 1.3.0, no longer supported | [optional] 
**edge_fog_latency** | **int** | **DEPRECATED** As of release 1.3.0, replaced by netChar latency | [optional] 
**edge_fog_latency_variation** | **int** | **DEPRECATED** As of release 1.3.0, replaced by netChar latencyVariation | [optional] 
**edge_fog_throughput** | **int** | **DEPRECATED** As of release 1.3.0, replaced by netChar throughput | [optional] 
**edge_fog_packet_loss** | **float** | **DEPRECATED** As of release 1.3.0, replaced by netChar packetLoss | [optional] 
**meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**user_meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**network_locations** | [**list[NetworkLocation]**](NetworkLocation.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

