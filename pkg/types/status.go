// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package types

// StatusCode is a Gateway/platform response status code. The success code is
// "0000"; all other documented codes are error conditions. The table is
// reproduced in docs/SPEC.md §6 from the legacy HStong OpenAPI documentation.
type StatusCode string

const (
	// StatusOK ("0000") indicates the request was processed successfully.
	StatusOK StatusCode = "0000"
	// StatusUnknownError ("1001") is an unknown system error.
	StatusUnknownError StatusCode = "1001"
	// StatusSignatureError ("1002") is a signature verification failure.
	StatusSignatureError StatusCode = "1002"
	// StatusEncryptionError ("1003") is a data encryption/decryption failure.
	StatusEncryptionError StatusCode = "1003"
	// StatusSocketNotInitialized ("1004") means the socket was not initialized.
	StatusSocketNotInitialized StatusCode = "1004"
	// StatusEndpointDeprecated ("1005") means the endpoint is deprecated.
	StatusEndpointDeprecated StatusCode = "1005"
	// StatusUserNotAuthorized ("1006") means the user is not yet authorized.
	StatusUserNotAuthorized StatusCode = "1006"
	// StatusDuplicateSubmit ("1007") means the request was submitted twice.
	StatusDuplicateSubmit StatusCode = "1007"
	// StatusCallFailed ("1008") means the call failed.
	StatusCallFailed StatusCode = "1008"
	// StatusEndpointNotFound ("1009") means the endpoint does not exist.
	StatusEndpointNotFound StatusCode = "1009"
	// StatusIllegalRequest ("1010") means the request was illegal.
	StatusIllegalRequest StatusCode = "1010"
	// StatusServiceBusy ("1011") means the service is busy; retry later.
	StatusServiceBusy StatusCode = "1011"
	// StatusNotLoggedIn ("1012") means the user is not logged in.
	StatusNotLoggedIn StatusCode = "1012"
	// StatusKickedOffline ("1013") means the session was displaced by another login.
	StatusKickedOffline StatusCode = "1013"
	// StatusLoginTimeout ("1014") means the login has timed out.
	StatusLoginTimeout StatusCode = "1014"
	// StatusCallTimeout ("1015") means the call timed out.
	StatusCallTimeout StatusCode = "1015"
	// StatusInvalidParam ("1016") means a call parameter was invalid.
	StatusInvalidParam StatusCode = "1016"
	// StatusConnectFailed ("1017") means the long connection could not be established.
	StatusConnectFailed StatusCode = "1017"
	// StatusReconnecting ("1018") means the client is reconnecting; retry later.
	StatusReconnecting StatusCode = "1018"
	// StatusFuturesLoginTimeout ("20033") means the futures trade login timed out.
	StatusFuturesLoginTimeout StatusCode = "20033"
	// StatusQueryProductInfoFailed ("40001") means querying product info failed.
	StatusQueryProductInfoFailed StatusCode = "40001"
	// StatusQueryContractInfoFailed ("40002") means querying contract info failed.
	StatusQueryContractInfoFailed StatusCode = "40002"
)

// IsSuccess reports whether c is the success code StatusOK ("0000").
func (c StatusCode) IsSuccess() bool {
	return c == StatusOK
}

// String returns a short human-readable description of c, or the raw code when
// the value is not one of the documented constants.
func (c StatusCode) String() string {
	switch c {
	case StatusOK:
		return "success"
	case StatusUnknownError:
		return "unknown system error"
	case StatusSignatureError:
		return "signature error"
	case StatusEncryptionError:
		return "data encryption error"
	case StatusSocketNotInitialized:
		return "socket not initialized"
	case StatusEndpointDeprecated:
		return "endpoint deprecated"
	case StatusUserNotAuthorized:
		return "user not authorized"
	case StatusDuplicateSubmit:
		return "duplicate submission"
	case StatusCallFailed:
		return "call failed"
	case StatusEndpointNotFound:
		return "endpoint not found"
	case StatusIllegalRequest:
		return "illegal request"
	case StatusServiceBusy:
		return "service busy, retry later"
	case StatusNotLoggedIn:
		return "not logged in"
	case StatusKickedOffline:
		return "session displaced"
	case StatusLoginTimeout:
		return "login timeout"
	case StatusCallTimeout:
		return "call timeout"
	case StatusInvalidParam:
		return "invalid parameter"
	case StatusConnectFailed:
		return "long-connection establishment failed"
	case StatusReconnecting:
		return "reconnecting, retry later"
	case StatusFuturesLoginTimeout:
		return "futures trade login timeout"
	case StatusQueryProductInfoFailed:
		return "query product info failed"
	case StatusQueryContractInfoFailed:
		return "query contract info failed"
	default:
		return string(c)
	}
}
