# StaDataRateSubscription

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Links** | [***AssocStaSubscriptionLinks**](AssocStaSubscription__links.md) |  | [optional] [default to null]
**CallbackReference** | **string** | URI exposed by the client on which to receive notifications | [optional] [default to null]
**ExpiryDeadline** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**NotificationEvent** | [***StaDataRateSubscriptionNotificationEvent**](StaDataRateSubscription_notificationEvent.md) |  | [optional] [default to null]
**NotificationPeriod** | **int32** | Set for periodic notification reporting. Value indicates the notification period in seconds. | [optional] [default to null]
**RequestTestNotification** | **bool** | Set to TRUE by the service consumer to request a test notification via HTTP on the callbackReference URI. | [optional] [default to null]
**StaId** | [**[]StaIdentity**](StaIdentity.md) | Identifier(s) to uniquely specify the target client station(s) for the subscription. | [default to null]
**SubscriptionType** | **string** | Shall be set to \&quot;StaDataRateSubscription\&quot;. | [default to null]
**WebsockNotifConfig** | [***WebsockNotifConfig**](WebsockNotifConfig.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

