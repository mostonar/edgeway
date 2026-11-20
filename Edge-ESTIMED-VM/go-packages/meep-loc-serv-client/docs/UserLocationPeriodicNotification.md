# UserLocationPeriodicNotification

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Links** | [***UserAreaNotificationLinks**](UserAreaNotification__links.md) |  | [default to null]
**AccessPointId** | **string** | The identity of the access point that the user is currently within.  See note 2. | [optional] [default to null]
**Address** | **string** | Address of user (e.g. ‘sip’ URI, ‘tel’ URI, ‘acr’ URI). | [optional] [default to null]
**CivicInfo** | [***CivicAddress**](CivicAddress.md) |  | [optional] [default to null]
**IsFinalNotification** | **bool** | Shall be set to true if it is a final notification. | [optional] [default to null]
**LocationInfo** | [***LocationInfo**](LocationInfo.md) |  | [optional] [default to null]
**NotificationType** | **string** | Shall be set to \&quot;UserLocationPeriodicNotification\&quot;. | [default to null]
**RelativeLocationInfo** | [***RelativeLocationInfo**](RelativeLocationInfo.md) |  | [optional] [default to null]
**Result** | [***NotificationResult**](NotificationResult.md) |  | [default to null]
**TimeStamp** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**ZoneId** | **string** | The identity of the zone that the user is currently within.  See note 2. | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

