# ZoneStatusSubscription

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Links** | [***Links**](_links.md) |  | [optional] [default to null]
**CallbackReference** | **string** | URI exposed by the client on which to receive notifications via HTTP. See note 1. | [optional] [default to null]
**ClientCorrelator** | **string** | A correlator that the client can use to tag this particular resource representation during a request to create a resource on the server. See note 2. | [optional] [default to null]
**ExpiryDeadline** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**LowerNumberOfUsersAPThreshold** | **int32** | Threshold number of users in an access point which if crossed downward shall cause a notification | [optional] [default to null]
**LowerNumberOfUsersZoneThreshold** | **int32** | Threshold number of users in a zone which if crossed downward shall cause a notification | [optional] [default to null]
**OperationStatus** | [**[]OperationStatus**](OperationStatus.md) | List of operation status values to generate notifications for (these apply to all access points within a zone). See note 3. | [optional] [default to null]
**ReportingCtrl** | [***ReportingCtrl**](ReportingCtrl.md) |  | [optional] [default to null]
**RequestTestNotification** | **bool** | Set to TRUE by the service consumer to request a test notification via HTTP on the callbackReference URI, as specified in ETSI GS MEC 009 [4], clause 6.12a. | [optional] [default to null]
**SubscriptionType** | **string** | Shall be set to \&quot;ZoneStatusSubscription\&quot;. | [default to null]
**UpperNumberOfUsersAPThreshold** | **int32** | Threshold number of users in an access point which if crossed upward shall cause a notification. | [optional] [default to null]
**UpperNumberOfUsersZoneThreshold** | **int32** | Threshold number of users in a zone which if crossed upward shall cause a notification. | [optional] [default to null]
**WebsockNotifConfig** | [***WebsockNotifConfig**](WebsockNotifConfig.md) |  | [optional] [default to null]
**ZoneId** | **string** | Identifier of zone (e.g. zone001) to monitor. | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

