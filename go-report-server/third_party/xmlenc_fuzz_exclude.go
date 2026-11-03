//go:build gofuzz

// This file replaces github.com/crewjam/saml/xmlenc/fuzz.go in production
// builds. The upstream file is a go-fuzz harness with no build constraint.
// It embeds a test RSA private key in a package-level variable, so any
// binary that imports the SAML stack links that key. The key is not an
// AcuGIS license key and is not used to decrypt SAML assertions.
//
// Normal builds do not set the gofuzz tag, so the compiler drops this
// replacement and the upstream harness together. The rest of xmlenc is
// unchanged.

package xmlenc
