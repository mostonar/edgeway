# ZoneStatusNotification

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Links** | [***UserAreaNotificationLinks**](UserAreaNotification__links.md) |  | [default to null]
**AccessPointId** | **string** | Identifier of an access point (e.g. ap01). Shall be included when userNumEvent related with access point or operationStatus is included. | [optional] [default to null]
**NotificationType** | **string** | Shall be set to \&quot;ZoneStatusNotification\&quot;. | [default to null]
**OperationStatus** | [***OperationStatus**](OperationStatus.md) |  | [optional] [default to null]
**TimeStamp** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**UserNumEvent** | **int32** | Shall be present when ZoneStatusSubscription includes upperNumberOfUsersZoneThreshold, lowerNumberOfUsersZoneThreshold, upperNumberOfUsersAPThreshold or lowerNumberOfUsersAPThreshold, and the number of users in a zone or an access point crosses the threshold defined in the subscription: 1 &#x3D; OVER_ZONE_UPPER_THD. 2 &#x3D; UNDER_ZONE_LOWER_THD. 3 &#x3D; OVER_AP_UPPER_THD. 4 &#x3D; UNDER_AP_LOWER_THD. | [optional] [default to null]
**ZoneId** | **string** | The identity of the zone.  | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

