# UserLocationEventNotification

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**AccessPointId** | **string** | The identity of the access point. For the events of \&quot;ENTERING_AREA_EVENT\&quot;, it indicates the access point that the user is currently within.  For the event of \&quot;LEAVING_AREA_EVENT\&quot;, it indicates the access point that the user used to be within. See note 2. | [optional] [default to null]
**Address** | **string** | Address of user (e.g. ‘sip’ URI, ‘tel’ URI, ‘acr’ URI). | [optional] [default to null]
**CivicInfo** | [***CivicAddress**](CivicAddress.md) |  | [optional] [default to null]
**LocationInfo** | [***LocationInfo**](LocationInfo.md) |  | [optional] [default to null]
**NotificationType** | **string** | Shall be set to \&quot;UserLocationEventNotification\&quot;. | [default to null]
**RelativeLocationInfo** | [***RelativeLocationInfo**](RelativeLocationInfo.md) |  | [optional] [default to null]
**TimeStamp** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**UserLocationEvent** | [***LocationEventType**](LocationEventType.md) |  | [default to null]
**ZoneId** | **string** | The identity of the zone.  For the events of \&quot;ENTERING_AREA_EVENT\&quot;, it is the zone that the user is currently within.  For the event of \&quot;LEAVING_AREA_EVENT\&quot;, it is the zone that the user used to be within. See note 2. | [optional] [default to null]
**Links** | [***UserAreaNotificationLinks**](UserAreaNotification__links.md) |  | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

