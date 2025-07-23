# ApiRefDataResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Prices** | Pointer to [**[]ApiRefDataResponsePrice**](ApiRefDataResponsePrice.md) |  | [optional] 
**Regions** | Pointer to **[]string** |  | [optional] 

## Methods

### NewApiRefDataResponse

`func NewApiRefDataResponse() *ApiRefDataResponse`

NewApiRefDataResponse instantiates a new ApiRefDataResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewApiRefDataResponseWithDefaults

`func NewApiRefDataResponseWithDefaults() *ApiRefDataResponse`

NewApiRefDataResponseWithDefaults instantiates a new ApiRefDataResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPrices

`func (o *ApiRefDataResponse) GetPrices() []ApiRefDataResponsePrice`

GetPrices returns the Prices field if non-nil, zero value otherwise.

### GetPricesOk

`func (o *ApiRefDataResponse) GetPricesOk() (*[]ApiRefDataResponsePrice, bool)`

GetPricesOk returns a tuple with the Prices field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPrices

`func (o *ApiRefDataResponse) SetPrices(v []ApiRefDataResponsePrice)`

SetPrices sets Prices field to given value.

### HasPrices

`func (o *ApiRefDataResponse) HasPrices() bool`

HasPrices returns a boolean if a field has been set.

### GetRegions

`func (o *ApiRefDataResponse) GetRegions() []string`

GetRegions returns the Regions field if non-nil, zero value otherwise.

### GetRegionsOk

`func (o *ApiRefDataResponse) GetRegionsOk() (*[]string, bool)`

GetRegionsOk returns a tuple with the Regions field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRegions

`func (o *ApiRefDataResponse) SetRegions(v []string)`

SetRegions sets Regions field to given value.

### HasRegions

`func (o *ApiRefDataResponse) HasRegions() bool`

HasRegions returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


