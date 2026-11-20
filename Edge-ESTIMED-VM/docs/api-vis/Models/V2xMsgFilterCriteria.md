# V2xMsgFilterCriteria

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**LocationInfo** | [**[]LocationInfo**](LocationInfo.md) | Location information to identify a cell of a base station or a particular geographical area to which the V2X message can be associated. | [optional] [default to null]
**MsgProtocolVersion** | **[]int32** | Protocol version of the V2X message (0..255), See note 3. | [optional] [default to null]
**MsgType** | **[]string** | Published V2X message type. Its value is defined by the standardization organization indicated by the attribute stdOrganization. See note 2. | [optional] [default to null]
**StdOrganization** | **string** | Standardization organization which defines the published V2X message type:  ETSI: European Telecommunications Standards Institute.  See note 1. | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

