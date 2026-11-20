# MbTransportInfo

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | **string** | The identifier of this transport as per ETSI GS MEC 011 [i.2]. | [default to null]
**Name** | **string** | The name of this transport as per ETSI GS MEC 011 [i.2]. | [default to null]
**Description** | **string** | Human-readable description of this transport as per ETSI GS MEC 011 [i.2]. | [optional] [default to null]
**Type_** | [***TransportType**](TransportType.md) |  | [default to null]
**Protocol** | **string** | The name of the protocol used. Being the transport of MB_TOPIC_BASED type, this attribute should be typically set to \&quot;MQTT\&quot; or \&quot;AMQP.\&quot; | [default to null]
**Version** | **string** | The version of the protocol used as per ETSI GS MEC 011 [i.2]. | [default to null]
**Endpoint** | [***EndPointInfo**](EndPointInfo.md) |  | [default to null]
**Security** | [***SecurityInfo**](SecurityInfo.md) |  | [default to null]
**ImplSpecificInfo** | [***ImplSpecificInfo**](ImplSpecificInfo.md) |  | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

