# Process

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique process ID | [optional] 
**name** | **str** | Process name | [optional] 
**type** | **str** | Process type | [optional] 
**is_external** | **bool** | true: process is external to MEEP false: process is internal to MEEP | [optional] 
**image** | **str** | Docker image to deploy inside MEEP | [optional] 
**environment** | **str** | Environment variables using the format NAME&#x3D;\&quot;value\&quot;,NAME&#x3D;\&quot;value\&quot;,NAME&#x3D;\&quot;value\&quot; | [optional] 
**command_arguments** | **str** | Arguments to command executable | [optional] 
**command_exe** | **str** | Executable to invoke at container start up | [optional] 
**service_config** | [**ServiceConfig**](ServiceConfig.md) |  | [optional] 
**gpu_config** | [**GpuConfig**](GpuConfig.md) |  | [optional] 
**memory_config** | [**MemoryConfig**](MemoryConfig.md) |  | [optional] 
**cpu_config** | [**CpuConfig**](CpuConfig.md) |  | [optional] 
**external_config** | [**ExternalConfig**](ExternalConfig.md) |  | [optional] 
**status** | **str** | Process status | [optional] 
**user_chart_location** | **str** | Chart location for the deployment of the chart provided by the user | [optional] 
**user_chart_alternate_values** | **str** | Chart values.yaml file location for the deployment of the chart provided by the user | [optional] 
**user_chart_group** | **str** | Chart supplemental information related to the group (service) | [optional] 
**meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**user_meta** | **dict(str, str)** | Key/Value Pair Map (string, string) | [optional] 
**net_char** | [**NetworkCharacteristics**](NetworkCharacteristics.md) |  | [optional] 
**app_latency** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latency | [optional] 
**app_latency_variation** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar latencyVariation | [optional] 
**app_throughput** | **int** | **DEPRECATED** As of release 1.5.0, replaced by netChar throughputUl and throughputDl | [optional] 
**app_packet_loss** | **float** | **DEPRECATED** As of release 1.5.0, replaced by netChar packetLoss | [optional] 
**placement_id** | **str** | Identifier used for process placement in AdvantEDGE cluster | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

