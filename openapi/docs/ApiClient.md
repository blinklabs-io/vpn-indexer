# ApiClient

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Expiration** | Pointer to **string** |  | [optional] 
**Id** | Pointer to **string** |  | [optional] 
**Region** | Pointer to **string** |  | [optional] 

## Methods

### NewApiClient

`func NewApiClient() *ApiClient`

NewApiClient instantiates a new ApiClient object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewApiClientWithDefaults

`func NewApiClientWithDefaults() *ApiClient`

NewApiClientWithDefaults instantiates a new ApiClient object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetExpiration

`func (o *ApiClient) GetExpiration() string`

GetExpiration returns the Expiration field if non-nil, zero value otherwise.

### GetExpirationOk

`func (o *ApiClient) GetExpirationOk() (*string, bool)`

GetExpirationOk returns a tuple with the Expiration field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpiration

`func (o *ApiClient) SetExpiration(v string)`

SetExpiration sets Expiration field to given value.

### HasExpiration

`func (o *ApiClient) HasExpiration() bool`

HasExpiration returns a boolean if a field has been set.

### GetId

`func (o *ApiClient) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *ApiClient) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *ApiClient) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *ApiClient) HasId() bool`

HasId returns a boolean if a field has been set.

### GetRegion

`func (o *ApiClient) GetRegion() string`

GetRegion returns the Region field if non-nil, zero value otherwise.

### GetRegionOk

`func (o *ApiClient) GetRegionOk() (*string, bool)`

GetRegionOk returns a tuple with the Region field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegion

`func (o *ApiClient) SetRegion(v string)`

SetRegion sets Region field to given value.

### HasRegion

`func (o *ApiClient) HasRegion() bool`

HasRegion returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


