/*
vpn-indexer

NABU VPN indexer API

API version: v0
Contact: support@blinklabs.io
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"encoding/json"
)

// checks if the ApiRefDataResponse type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &ApiRefDataResponse{}

// ApiRefDataResponse struct for ApiRefDataResponse
type ApiRefDataResponse struct {
	Prices  []ApiRefDataResponsePrice `json:"prices,omitempty"`
	Regions []string                  `json:"regions,omitempty"`
}

// NewApiRefDataResponse instantiates a new ApiRefDataResponse object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewApiRefDataResponse() *ApiRefDataResponse {
	this := ApiRefDataResponse{}
	return &this
}

// NewApiRefDataResponseWithDefaults instantiates a new ApiRefDataResponse object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewApiRefDataResponseWithDefaults() *ApiRefDataResponse {
	this := ApiRefDataResponse{}
	return &this
}

// GetPrices returns the Prices field value if set, zero value otherwise.
func (o *ApiRefDataResponse) GetPrices() []ApiRefDataResponsePrice {
	if o == nil || IsNil(o.Prices) {
		var ret []ApiRefDataResponsePrice
		return ret
	}
	return o.Prices
}

// GetPricesOk returns a tuple with the Prices field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ApiRefDataResponse) GetPricesOk() ([]ApiRefDataResponsePrice, bool) {
	if o == nil || IsNil(o.Prices) {
		return nil, false
	}
	return o.Prices, true
}

// HasPrices returns a boolean if a field has been set.
func (o *ApiRefDataResponse) HasPrices() bool {
	if o != nil && !IsNil(o.Prices) {
		return true
	}

	return false
}

// SetPrices gets a reference to the given []ApiRefDataResponsePrice and assigns it to the Prices field.
func (o *ApiRefDataResponse) SetPrices(v []ApiRefDataResponsePrice) {
	o.Prices = v
}

// GetRegions returns the Regions field value if set, zero value otherwise.
func (o *ApiRefDataResponse) GetRegions() []string {
	if o == nil || IsNil(o.Regions) {
		var ret []string
		return ret
	}
	return o.Regions
}

// GetRegionsOk returns a tuple with the Regions field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ApiRefDataResponse) GetRegionsOk() ([]string, bool) {
	if o == nil || IsNil(o.Regions) {
		return nil, false
	}
	return o.Regions, true
}

// HasRegions returns a boolean if a field has been set.
func (o *ApiRefDataResponse) HasRegions() bool {
	if o != nil && !IsNil(o.Regions) {
		return true
	}

	return false
}

// SetRegions gets a reference to the given []string and assigns it to the Regions field.
func (o *ApiRefDataResponse) SetRegions(v []string) {
	o.Regions = v
}

func (o ApiRefDataResponse) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o ApiRefDataResponse) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Prices) {
		toSerialize["prices"] = o.Prices
	}
	if !IsNil(o.Regions) {
		toSerialize["regions"] = o.Regions
	}
	return toSerialize, nil
}

type NullableApiRefDataResponse struct {
	value *ApiRefDataResponse
	isSet bool
}

func (v NullableApiRefDataResponse) Get() *ApiRefDataResponse {
	return v.value
}

func (v *NullableApiRefDataResponse) Set(val *ApiRefDataResponse) {
	v.value = val
	v.isSet = true
}

func (v NullableApiRefDataResponse) IsSet() bool {
	return v.isSet
}

func (v *NullableApiRefDataResponse) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableApiRefDataResponse(
	val *ApiRefDataResponse,
) *NullableApiRefDataResponse {
	return &NullableApiRefDataResponse{value: val, isSet: true}
}

func (v NullableApiRefDataResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableApiRefDataResponse) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
