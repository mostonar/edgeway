# UserAreaSubscription

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Links** | [***Links**](_links.md) |  | [optional] [default to null]
**AddressList** | **[]string** | List of the users to be monitored.  | [default to null]
**AreaDefine** | [***AreaInfo**](AreaInfo.md) |  | [default to null]
**CallbackReference** | **string** | URI exposed by the client on which to receive notifications via HTTP. See note 1. | [optional] [default to null]
**ClientCorrelator** | **string** | A correlator that the client can use to tag this particular resource representation during a request to create a resource on the server. See note 2. | [optional] [default to null]
**ExpiryDeadline** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**LocationEventCriteria** | [**[]LocationEventType**](LocationEventType.md) | List of user event values to generate notifications for (these apply to address specified).  | [optional] [default to null]
**ReportingCtrl** | [***ReportingCtrl**](ReportingCtrl.md) |  | [optional] [default to null]
**ReportingLocationReq** | **bool** | This IE shall be set to true if a location estimate is required for each event report. | [optional] [default to null]
**RequestTestNotification** | **bool** | Set to TRUE by the service consumer to request a test notification via HTTP on the callbackReference URI, as specified in ETSI GS MEC 009 [4], clause 6.12a. | [optional] [default to null]
**SubscriptionType** | **string** | Shall be set to \&quot;UserAreaSubscription\&quot;. | [default to null]
**TrackingAccuracy** | **float32** | Number of meters of acceptable error. | [default to null]
**WebsockNotifConfig** | [***WebsockNotifConfig**](WebsockNotifConfig.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

