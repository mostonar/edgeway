# NetworkLocation

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique network location ID | [optional] 
**name** | **str** | Network location name | [optional] 
**type** | **str** | Network location type | [optional] 
**net_char** | [**NetworkCharacteristics**](NetworkCharacteristics.md) |  | [optional] 
**terminal_link_latency** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latency | [optional] 
**terminal_link_latency_variation** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latencyVariation | [optional] 
**terminal_link_throughput** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar throughputUl and throughputDl | [optional] 
**terminal_link_packet_loss** | **float** | **DEPRECATED** As of release 1.5.0, replaced by netChar packetLoss | [optional] 
**meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**user_meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**cellular_poa_config** | [**CellularPoaConfig**](CellularPoaConfig.md) |  | [optional] 
**poa4_g_config** | [**Poa4GConfig**](Poa4GConfig.md) |  | [optional] 
**poa5_g_config** | [**Poa5GConfig**](Poa5GConfig.md) |  | [optional] 
**poa_wifi_config** | [**PoaWifiConfig**](PoaWifiConfig.md) |  | [optional] 
**geo_data** | [**GeoData**](GeoData.md) |  | [optional] 
**physical_locations** | [**list[PhysicalLocation]**](PhysicalLocation.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

