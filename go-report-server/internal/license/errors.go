// Package license verifies AcuGIS Report Server Professional ACL1 entitlements.
// The licensing server signs tokens. This package only holds the public
// verification key and never an activation key, admin token, or private key.
package license

import "errors"

// Error is a stable verification or licensing failure.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

const (
	CodeInvalid         = "invalid"
	CodeSignature       = "signature"
	CodeMismatch        = "installation_mismatch"
	CodeExpired         = "expired"
	CodeIdentityInvalid = "identity_invalid"
	CodeUnknownProduct  = "unknown_product"
	CodeWrongProduct    = "wrong_product"
	CodeVerifier        = "verifier"
	CodeMissing         = "missing"
)

// As returns a license error.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func errInvalid() error {
	return &Error{Code: CodeInvalid, Message: "AcuGIS license is invalid"}
}

func errSignature() error {
	return &Error{Code: CodeSignature, Message: "AcuGIS license signature is invalid"}
}

func errMismatch() error {
	return &Error{Code: CodeMismatch, Message: "AcuGIS license is for a different installation"}
}

func errExpired() error {
	return &Error{Code: CodeExpired, Message: "AcuGIS license has expired"}
}

func errIdentityInvalid() error {
	return &Error{Code: CodeIdentityInvalid, Message: "AcuGIS installation identity is invalid"}
}

func errUnknownProduct() error {
	return &Error{Code: CodeUnknownProduct, Message: "AcuGIS product is not supported"}
}

func errWrongProduct() error {
	return &Error{Code: CodeWrongProduct, Message: "AcuGIS product does not match"}
}

func errVerifier() error {
	return &Error{Code: CodeVerifier, Message: "AcuGIS license verification is not configured"}
}

func errMissing() error {
	return &Error{Code: CodeMissing, Message: "AcuGIS license is not installed"}
}
