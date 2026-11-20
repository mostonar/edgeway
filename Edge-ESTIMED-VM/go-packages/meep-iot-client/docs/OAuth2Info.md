# OAuth2Info

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**GrantTypes** | **[]string** | List of supported OAuth 2.0 grant types. Each entry shall be one of the following permitted values: - OAUTH2_AUTHORIZATION_CODE: Authorization code grant type - OAUTH2_IMPLICIT_GRANT: Implicit grant type - OAUTH2_RESOURCE_OWNER: Resource owner password credentials grant type - OAUTH2_CLIENT_CREDENTIALS: Client credentials grant type Only the value \&quot;OAUTH2_CLIENT_CREDENTIALS\&quot; is supported in the present document.  | [default to null]
**TokenEndpoint** | **string** | The token endpoint. Shall be present unless the grant type is OAUTH2_IMPLICIT_GRANT. | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

