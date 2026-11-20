# SystemUpdateNotificationSubscription

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**SubscriptionType** | **string** | Shall be set to \&quot;SystemUpdateNotificationSubscription\&quot;. | [default to null]
**CallbackReference** | **string** | URI selected by the MEC orchestrator to receive notifications on the subscribed MEC system information updates in the MEC federation. This shall be included in both the request and the response. | [default to null]
**Links** | [***Links**](links.md) |  | [optional] [default to null]
**SystemId** | **[]string** | Identifier(s) to uniquely specify the target MEC system(s) for the subscription. If absent, the subscription should include all MEC systems in the MEC federation. | [optional] [default to null]
**ExpiryDeadline** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

