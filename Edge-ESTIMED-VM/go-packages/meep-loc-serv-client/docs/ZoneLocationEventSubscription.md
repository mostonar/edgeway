# ZoneLocationEventSubscription

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Links** | [***Links**](_links.md) |  | [optional] [default to null]
**AddressList** | **[]string** | List of the users to be monitored. If not present, all the users need to be monitored. | [optional] [default to null]
**CallbackReference** | **string** | URI exposed by the client on which to receive notifications via HTTP. See note 1. | [optional] [default to null]
**ClientCorrelator** | **string** | A correlator that the client can use to tag this particular resource representation during a request to create a resource on the server. See note 2. | [optional] [default to null]
**ExpiryDeadline** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**LocationEventCriteria** | [**[]LocationEventType**](LocationEventType.md) | List of user event values to generate notifications for.  | [optional] [default to null]
**ReportingCtrl** | [***ReportingCtrl**](ReportingCtrl.md) |  | [optional] [default to null]
**RequestTestNotification** | **bool** | Set to TRUE by the service consumer to request a test notification via HTTP on the callbackReference URI, as specified in ETSI GS MEC 009 [4], clause 6.12a. | [optional] [default to null]
**SubscriptionType** | **string** | Shall be set to \&quot;ZoneLocationEventSubscription\&quot;. | [default to null]
**WebsockNotifConfig** | [***WebsockNotifConfig**](WebsockNotifConfig.md) |  | [optional] [default to null]
**ZoneId** | **string** | Identifier of zone (e.g. zone001) to monitor. | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

