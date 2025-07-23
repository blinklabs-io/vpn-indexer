# \DefaultAPI

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ApiClientAvailablePost**](DefaultAPI.md#ApiClientAvailablePost) | **Post** /api/client/available | ClientAvailable
[**ApiClientListPost**](DefaultAPI.md#ApiClientListPost) | **Post** /api/client/list | ClientList
[**ApiClientProfilePost**](DefaultAPI.md#ApiClientProfilePost) | **Post** /api/client/profile | ClientProfile
[**ApiRefdataGet**](DefaultAPI.md#ApiRefdataGet) | **Get** /api/refdata | RefData
[**ApiTxSignupPost**](DefaultAPI.md#ApiTxSignupPost) | **Post** /api/tx/signup | TxSignup



## ApiClientAvailablePost

> string ApiClientAvailablePost(ctx).ClientAvailableRequest(clientAvailableRequest).Execute()

ClientAvailable



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/blinklabs-io/vpn-indexer/openapi"
)

func main() {
	clientAvailableRequest := *openapiclient.NewApiClientAvailableRequest() // ApiClientAvailableRequest | Client Available Request

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.ApiClientAvailablePost(context.Background()).ClientAvailableRequest(clientAvailableRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.ApiClientAvailablePost``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `ApiClientAvailablePost`: string
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.ApiClientAvailablePost`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiApiClientAvailablePostRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **clientAvailableRequest** | [**ApiClientAvailableRequest**](ApiClientAvailableRequest.md) | Client Available Request | 

### Return type

**string**

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: */*

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## ApiClientListPost

> []ApiClient ApiClientListPost(ctx).ClientListRequest(clientListRequest).Execute()

ClientList



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/blinklabs-io/vpn-indexer/openapi"
)

func main() {
	clientListRequest := *openapiclient.NewApiClientListRequest() // ApiClientListRequest | List Request

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.ApiClientListPost(context.Background()).ClientListRequest(clientListRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.ApiClientListPost``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `ApiClientListPost`: []ApiClient
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.ApiClientListPost`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiApiClientListPostRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **clientListRequest** | [**ApiClientListRequest**](ApiClientListRequest.md) | List Request | 

### Return type

[**[]ApiClient**](ApiClient.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## ApiClientProfilePost

> ApiClientProfilePost(ctx).ClientProfileRequest(clientProfileRequest).Execute()

ClientProfile



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/blinklabs-io/vpn-indexer/openapi"
)

func main() {
	clientProfileRequest := *openapiclient.NewApiClientProfileRequest() // ApiClientProfileRequest | Profile Request

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	r, err := apiClient.DefaultAPI.ApiClientProfilePost(context.Background()).ClientProfileRequest(clientProfileRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.ApiClientProfilePost``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiApiClientProfilePostRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **clientProfileRequest** | [**ApiClientProfileRequest**](ApiClientProfileRequest.md) | Profile Request | 

### Return type

 (empty response body)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: */*

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## ApiRefdataGet

> ApiRefDataResponse ApiRefdataGet(ctx).Execute()

RefData



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/blinklabs-io/vpn-indexer/openapi"
)

func main() {

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.ApiRefdataGet(context.Background()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.ApiRefdataGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `ApiRefdataGet`: ApiRefDataResponse
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.ApiRefdataGet`: %v\n", resp)
}
```

### Path Parameters

This endpoint does not need any parameter.

### Other Parameters

Other parameters are passed through a pointer to a apiApiRefdataGetRequest struct via the builder pattern


### Return type

[**ApiRefDataResponse**](ApiRefDataResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## ApiTxSignupPost

> ApiTxSignupResponse ApiTxSignupPost(ctx).TxSignupRequest(txSignupRequest).Execute()

TxSignup



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/blinklabs-io/vpn-indexer/openapi"
)

func main() {
	txSignupRequest := *openapiclient.NewApiTxSignupRequest() // ApiTxSignupRequest | Signup Request

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.DefaultAPI.ApiTxSignupPost(context.Background()).TxSignupRequest(txSignupRequest).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `DefaultAPI.ApiTxSignupPost``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `ApiTxSignupPost`: ApiTxSignupResponse
	fmt.Fprintf(os.Stdout, "Response from `DefaultAPI.ApiTxSignupPost`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiApiTxSignupPostRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **txSignupRequest** | [**ApiTxSignupRequest**](ApiTxSignupRequest.md) | Signup Request | 

### Return type

[**ApiTxSignupResponse**](ApiTxSignupResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

