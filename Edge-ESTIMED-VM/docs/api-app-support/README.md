# Documentation for AdvantEDGE MEC Application Support API

<a name="documentation-for-api-endpoints"></a>
## Documentation for API Endpoints

All URIs are relative to *https://localhost/sandboxname/mec_app_support/v2*

Class | Method | HTTP request | Description
------------ | ------------- | ------------- | -------------
*MecAppSupportApi* | [**ApplicationsConfirmReadyPOST**](docs/MecAppSupportApi.md#applicationsconfirmreadypost) | **Post** /applications/{appInstanceId}/confirm_ready | 
*MecAppSupportApi* | [**ApplicationsConfirmTerminationPOST**](docs/MecAppSupportApi.md#applicationsconfirmterminationpost) | **Post** /applications/{appInstanceId}/confirm_termination | 
*MecAppSupportApi* | [**ApplicationsRegistrationDELETE**](docs/MecAppSupportApi.md#applicationsregistrationdelete) | **Delete** /registrations/{appInstanceId} |
*MecAppSupportApi* | [**ApplicationsRegistrationGET**](docs/MecAppSupportApi.md#applicationsregistrationget) | **Get** /registrations/{appInstanceId} |
*MecAppSupportApi* | [**ApplicationsRegistrationPUT**](docs/MecAppSupportApi.md#applicationsregistrationput) | **Put** /registrations/{appInstanceId} |
*MecAppSupportApi* | [**ApplicationsRegistrationsPOST**](docs/MecAppSupportApi.md#applicationsregistrationspost) | **Post** /registrations |
*MecAppSupportApi* | [**ApplicationsSubscriptionDELETE**](docs/MecAppSupportApi.md#applicationssubscriptiondelete) | **Delete** /applications/{appInstanceId}/subscriptions/{subscriptionId} | 
*MecAppSupportApi* | [**ApplicationsSubscriptionGET**](docs/MecAppSupportApi.md#applicationssubscriptionget) | **Get** /applications/{appInstanceId}/subscriptions/{subscriptionId} | 
*MecAppSupportApi* | [**ApplicationsSubscriptionsGET**](docs/MecAppSupportApi.md#applicationssubscriptionsget) | **Get** /applications/{appInstanceId}/subscriptions | 
*MecAppSupportApi* | [**ApplicationsSubscriptionsPOST**](docs/MecAppSupportApi.md#applicationssubscriptionspost) | **Post** /applications/{appInstanceId}/subscriptions | 
*MecAppSupportApi* | [**TimingCapsGET**](docs/MecAppSupportApi.md#timingcapsget) | **Get** /timing/timing_caps | 
*MecAppSupportApi* | [**TimingCurrentTimeGET**](docs/MecAppSupportApi.md#timingcurrenttimeget) | **Get** /timing/current_time | 
*UnsupportedApi* | [**ApplicationsDnsRuleGET**](docs/UnsupportedApi.md#applicationsdnsruleget) | **Get** /applications/{appInstanceId}/dns_rules/{dnsRuleId} | 
*UnsupportedApi* | [**ApplicationsDnsRulePUT**](docs/UnsupportedApi.md#applicationsdnsruleput) | **Put** /applications/{appInstanceId}/dns_rules/{dnsRuleId} | 
*UnsupportedApi* | [**ApplicationsDnsRulesGET**](docs/UnsupportedApi.md#applicationsdnsrulesget) | **Get** /applications/{appInstanceId}/dns_rules | 
*UnsupportedApi* | [**ApplicationsTrafficRuleGET**](docs/UnsupportedApi.md#applicationstrafficruleget) | **Get** /applications/{appInstanceId}/traffic_rules/{trafficRuleId} | 
*UnsupportedApi* | [**ApplicationsTrafficRulePUT**](docs/UnsupportedApi.md#applicationstrafficruleput) | **Put** /applications/{appInstanceId}/traffic_rules/{trafficRuleId} | 
*UnsupportedApi* | [**ApplicationsTrafficRulesGET**](docs/UnsupportedApi.md#applicationstrafficrulesget) | **Get** /applications/{appInstanceId}/traffic_rules | 

<a name="documentation-for-models"></a>
## Documentation for Models

 - [AppInfo](docs/AppInfo.md)
 - [AppProfile](docs/AppProfile.md)
 - [AppReadyConfirmation](docs/AppReadyConfirmation.md)
 - [AppTerminationConfirmation](docs/AppTerminationConfirmation.md)
 - [AppTerminationNotification](docs/AppTerminationNotification.md)
 - [AppTerminationNotificationLinks](docs/AppTerminationNotificationLinks.md)
 - [AppTerminationNotificationSubscription](docs/AppTerminationNotificationSubscription.md)
 - [CategoryRef](docs/CategoryRef.md)
 - [CurrentTime](docs/CurrentTime.md)
 - [DestinationInterface](docs/DestinationInterface.md)
 - [DestinationInterfaceInterfaceType](docs/DestinationInterfaceInterfaceType.md)
 - [DnsRule](docs/DnsRule.md)
 - [DnsRuleIpAddressType](docs/DnsRuleIpAddressType.md)
 - [DnsRuleState](docs/DnsRuleState.md)
 - [EndPointInfoAddress](docs/EndPointInfoAddress.md)
 - [EndPointInfoAddresses](docs/EndPointInfoAddresses.md)
 - [EndPointInfoAlternative](docs/EndPointInfoAlternative.md)
 - [EndPointInfoFqdn](docs/EndPointInfoFqdn.md)
 - [EndPointInfoUris](docs/EndPointInfoUris.md)
 - [FeatureDependency](docs/FeatureDependency.md)
 - [LinkType](docs/LinkType.md)
 - [LinkTypeConfirmTermination](docs/LinkTypeConfirmTermination.md)
 - [MecAppSuptApiSubscriptionLinkList](docs/MecAppSuptApiSubscriptionLinkList.md)
 - [MecAppSuptApiSubscriptionLinkListLinks](docs/MecAppSuptApiSubscriptionLinkListLinks.md)
 - [MecAppSuptApiSubscriptionLinkListSubscription](docs/MecAppSuptApiSubscriptionLinkListSubscription.md)
 - [OneOfAppInfoEndpoint](docs/OneOfAppInfoEndpoint.md)
 - [OneOfAppProfileEndPt](docs/OneOfAppProfileEndPt.md)
 - [OperationActionType](docs/OperationActionType.md)
 - [ProblemDetails](docs/ProblemDetails.md)
 - [SecurityInfo](docs/SecurityInfo.md)
 - [SecurityInfoOAuth2Info](docs/SecurityInfoOAuth2Info.md)
 - [SecurityInfoOAuth2InfoGrantType](docs/SecurityInfoOAuth2InfoGrantType.md)
 - [Self](docs/Self.md)
 - [SerializerType](docs/SerializerType.md)
 - [ServiceDependency](docs/ServiceDependency.md)
 - [TimeSourceStatus](docs/TimeSourceStatus.md)
 - [TimingCaps](docs/TimingCaps.md)
 - [TimingCapsNtpServers](docs/TimingCapsNtpServers.md)
 - [TimingCapsNtpServersAuthenticationOption](docs/TimingCapsNtpServersAuthenticationOption.md)
 - [TimingCapsNtpServersNtpServerAddrType](docs/TimingCapsNtpServersNtpServerAddrType.md)
 - [TimingCapsPtpMasters](docs/TimingCapsPtpMasters.md)
 - [TimingCapsTimeStamp](docs/TimingCapsTimeStamp.md)
 - [TrafficFilter](docs/TrafficFilter.md)
 - [TrafficRule](docs/TrafficRule.md)
 - [TrafficRuleAction](docs/TrafficRuleAction.md)
 - [TrafficRuleFilterType](docs/TrafficRuleFilterType.md)
 - [TrafficRuleState](docs/TrafficRuleState.md)
 - [TransportDependency](docs/TransportDependency.md)
 - [TransportDescriptor](docs/TransportDescriptor.md)
 - [TransportType](docs/TransportType.md)
 - [TunnelInfo](docs/TunnelInfo.md)
 - [TunnelInfoTunnelType](docs/TunnelInfoTunnelType.md)


<a name="documentation-for-authorization"></a>
## Documentation for Authorization

All endpoints do not require authorization.
