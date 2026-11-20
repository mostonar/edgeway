# DeviceInfo

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**DeviceAuthenticationInfo** | **string** | Information needed for secondary authentication of the IoT device to the data network - see ETSI TS 129 561 [i.4] and ETSI TS 129 061 [i.5] for 5G and LTE procedures, respectively. This attribute is implementation dependent and should be logically linked to the identifiers of the IoT device listed hereafter. | [default to null]
**DeviceMetadata** | [**[]KeyValuePair**](KeyValuePair.md) | Additional information about the IoT device. This attribute is implementation dependent and may be expressed as an array of keyvalue pairs.  | [optional] [default to null]
**Gpsi** | **string** | GPSI of the IoT device if 5G-enabled (see note 1). | [optional] [default to null]
**Pei** | **string** | PEI of the IoT device if 5G-enabled (see note 1). | [optional] [default to null]
**Supi** | **string** | SUPI of the IoT device if 5G-enabled (see note 1). | [optional] [default to null]
**Msisdn** | **string** | MSISDN of the IoT device if LTE-enabled (see note 1). | [optional] [default to null]
**Imei** | **string** | IMEI of the IoT device if LTE-enabled (see note 1). | [optional] [default to null]
**Imsi** | **string** | IMSI of the IoT device if LTE-enabled (see note 1). | [optional] [default to null]
**Iccid** | **string** | ICCID of the IoT device (see note 1). | [optional] [default to null]
**DeviceId** | **string** | Human-readable identifier of the IoT device. | [default to null]
**RequestedMecTrafficRule** | [**[]TrafficRuleDescriptor**](TrafficRuleDescriptor.md) | MEC traffic rules the IoT device is requested to be associated to (see note 2). The data type definition is as per ETSI GS MEC 010-2 [i.6]. | [optional] [default to null]
**RequestedIotPlatformId** | **string** | IoT platform to which the IoT device is requested to be associated to (see note 2). | [optional] [default to null]
**RequestedUserTransportId** | **string** | User transport to which the IoT device is requested to be associated to (see note 2). | [optional] [default to null]
**DeviceSpecificMessageFormats** | [***DeviceSpecificMessageFormats**](DeviceSpecificMessageFormats.md) |  | [optional] [default to null]
**DownlinkInfo** | [***DownlinkInfo**](DownlinkInfo.md) |  | [optional] [default to null]
**ClientCertificate** | **string** | Client-side SSL/TLS certificate to be used by the MEC IoTS to interact with the user transport provided by the associated IoT platform in case the MEC IoTS acts on behalf of the IoT device. | [optional] [default to null]
**Enabled** | **bool** | Indication whether the IoT device has a valid associated traffic rule (TRUE) or not (FALSE). See note 3. | [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

