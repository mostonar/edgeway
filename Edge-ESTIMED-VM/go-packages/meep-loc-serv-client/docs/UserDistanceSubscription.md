# UserDistanceSubscription

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Links** | [***Links**](_links.md) |  | [optional] [default to null]
**CallbackReference** | **string** | URI exposed by the client on which to receive notifications via HTTP. See note 1. | [optional] [default to null]
**CheckImmediate** | **bool** | Check location immediately after establishing notification. | [default to null]
**ClientCorrelator** | **string** | A correlator that the client can use to tag this particular resource representation during a request to create a resource on the server. See note 2. | [optional] [default to null]
**Criteria** | [***DistanceCriteria**](DistanceCriteria.md) |  | [default to null]
**Distance** | **float32** | Distance between users that shall be monitored. The unit is meter. | [default to null]
**ExpiryDeadline** | [***TimeStamp**](TimeStamp.md) |  | [optional] [default to null]
**MonitoredAddress** | **[]string** | Contains addresses of users to monitor (e.g., ‘sip’ URI, ‘tel’ URI, ‘acr’ URI). Reference to a group could be provided here if supported by implementation. If the ReferenceAddress is specified, then the distance between each monitored user and reference user(s) will be monitored. If the ReferenceAddress is not present, then the distance between each pair of the monitored users will be monitored. Note that in that case there shall be at least two addresses specified here. | [default to null]
**ReferenceAddress** | **[]string** | If specified, indicates address of each user that will be used as reference users from which the distances towards monitored users indicated in the Addresses will be monitored (e.g., ‘sip’ URI, ‘tel’ URI, ‘acr’ URI). Reference to a group could be provided here if supported by implementation. | [optional] [default to null]
**ReportingCtrl** | [***ReportingCtrl**](ReportingCtrl.md) |  | [optional] [default to null]
**RequestTestNotification** | **bool** | Set to TRUE by the service consumer to request a test notification via HTTP on the callbackReference URI, as specified in ETSI GS MEC 009 [4], clause 6.12a. | [optional] [default to null]
**SubscriptionType** | **string** | Shall be set to \&quot;UserDistanceSubscription\&quot;. | [default to null]
**TrackingAccuracy** | **float32** | Number of meters of acceptable error in tracking distance. | [default to null]
**WebsockNotifConfig** | [***WebsockNotifConfig**](WebsockNotifConfig.md) |  | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

