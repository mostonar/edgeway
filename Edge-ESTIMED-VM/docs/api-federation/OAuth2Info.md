# OAuth2Info

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**GrantTypes** | **[]string** | \&quot;List of supported OAuth 2.0 grant types.\\nEach entry shall be one of the following permitted values:\\nOAUTH2_AUTHORIZATION_CODE (Authorization code grant type)\\nOAUTH2_IMPLICIT_GRANT\\n \\t(Implicit grant type)\\nOAUTH2_RESOURCE_OWNER\\n\\t(Resource owner password credentials grant type) \\nOAUTH2_CLIENT_CREDENTIALS\\n\\t(Client credentials grant type)\\nOnly the value \\\&quot;OAUTH2_CLIENT_CREDENTIALS\\\&quot; is supported in the present document. \&quot;  | [default to null]
**TokenEndpoint** | **string** | The token endpoint. Shall be present unless the grant type is OAUTH2_IMPLICIT_GRANT. | [optional] [default to null]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

