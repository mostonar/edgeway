# PhysicalLocation

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique physical location ID | [optional] 
**name** | **str** | Physical location name | [optional] 
**type** | **str** | Physical location type | [optional] 
**is_external** | **bool** | true: Physical location is external to MEEP false: Physical location is internal to MEEP | [optional] 
**geo_data** | [**GeoData**](GeoData.md) |  | [optional] 
**network_locations_in_range** | **list[str]** |  | [optional] 
**connected** | **bool** | true: Physical location has network connectivity false: Physical location has no network connectivity | [optional] 
**wireless** | **bool** | true: Physical location uses a wireless connection false: Physical location uses a wired connection | [optional] 
**wireless_type** | **str** | Prioritized, comma-separated list of supported wireless connection types. Default priority if not specififed is &#x27;wifi,5g,4g,other&#x27;. Wireless connection types: - 4g - 5g - wifi - other | [optional] 
**data_network** | [**DNConfig**](DNConfig.md) |  | [optional] 
**meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**user_meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**processes** | [**list[Process]**](Process.md) |  | [optional] 
**net_char** | [**NetworkCharacteristics**](NetworkCharacteristics.md) |  | [optional] 
**link_latency** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latency | [optional] 
**link_latency_variation** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latencyVariation | [optional] 
**link_throughput** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar throughputUl and throughputDl | [optional] 
**link_packet_loss** | **float** | **DEPRECATED** As of release 1.5.0, replaced by netChar packetLoss | [optional] 
**mac_id** | **str** | Physical location MAC Address | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

