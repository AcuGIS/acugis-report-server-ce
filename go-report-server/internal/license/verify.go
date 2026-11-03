package license

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"time"
)

// Verifier checks a signed AcuGIS license token.
type Verifier struct {
	PublicKey ed25519.PublicKey
	Now       func() time.Time
}

// VerifiedLicense is a license that passed signature and identity validation.
// Expiry is enforced separately by ParseAndVerify.
type VerifiedLicense struct {
	LicenseID      string
	InstallationID string
	ProductCode    string
	IssuedAt       time.Time
	ExpiresAt      time.Time
	// LicenseType is informational. Entitlement checks ignore it.
	LicenseType string
}

// ParsePublicKeyHex decodes a 32-byte Ed25519 public key.
func ParsePublicKeyHex(raw string) (ed25519.PublicKey, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errVerifier()
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errVerifier()
	}
	return ed25519.PublicKey(b), nil
}

func (v *Verifier) now() time.Time {
	if v != nil && v.Now != nil {
		return v.Now().UTC()
	}
	return time.Now().UTC()
}

func (v *Verifier) publicKey() (ed25519.PublicKey, error) {
	if v == nil || len(v.PublicKey) != ed25519.PublicKeySize {
		return nil, errVerifier()
	}
	return v.PublicKey, nil
}

// ParseAndVerify parses a token, verifies the Ed25519 signature over
// ACL1.<payload-segment>, then requires report-server-professional,
// the local installation ID, and expires_at in the future.
func ParseAndVerify(token string, v *Verifier, installationID string) (*VerifiedLicense, error) {
	lic, err := ParseIdentity(token, v, installationID, ProductReportServerProfessional)
	if err != nil {
		return nil, err
	}
	if !v.now().Before(lic.ExpiresAt) {
		return nil, errExpired()
	}
	return lic, nil
}

// ParseIdentity verifies signature and identity without treating expiry as failure.
// Callers that need product use must still require now < expires_at.
func ParseIdentity(token string, v *Verifier, installationID, expectedProduct string) (*VerifiedLicense, error) {
	payloadB64, sigB64, err := ParseToken(token)
	if err != nil {
		return nil, err
	}
	payloadRaw, err := decodeB64(payloadB64)
	if err != nil {
		return nil, err
	}
	sig, err := decodeB64(sigB64)
	if err != nil {
		return nil, err
	}
	pub, err := v.publicKey()
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(pub, signedMessage(payloadB64), sig) {
		return nil, errSignature()
	}
	payload, err := parsePayload(payloadRaw)
	if err != nil {
		return nil, err
	}
	issuedAt, expiresAt, err := validatePayloadIdentity(payload, v.now(), installationID, expectedProduct)
	if err != nil {
		return nil, err
	}
	return &VerifiedLicense{
		LicenseID:      strings.TrimSpace(payload.LicenseID),
		InstallationID: strings.ToLower(strings.TrimSpace(payload.InstallationID)),
		ProductCode:    strings.TrimSpace(payload.ProductCode),
		IssuedAt:       issuedAt,
		ExpiresAt:      expiresAt,
		LicenseType:    InformationalLicenseType(payload.LicenseType),
	}, nil
}

// WithinRefreshGrace reports whether an identity-valid token may still be
// submitted to /v1/refresh. It does not grant Professional use.
func WithinRefreshGrace(lic *VerifiedLicense, now time.Time) bool {
	if lic == nil {
		return false
	}
	return now.UTC().Before(lic.ExpiresAt.Add(RefreshGracePeriod))
}

func validatePayloadIdentity(p Payload, now time.Time, installationID, expectedProduct string) (issuedAt, expiresAt time.Time, err error) {
	if p.SchemaVersion != SchemaVersion {
		return time.Time{}, time.Time{}, errInvalid()
	}
	if !validLicenseID(p.LicenseID) {
		return time.Time{}, time.Time{}, errInvalid()
	}
	licInstall := strings.ToLower(strings.TrimSpace(p.InstallationID))
	if !ValidInstallationID(licInstall) {
		return time.Time{}, time.Time{}, errInvalid()
	}
	want := strings.ToLower(strings.TrimSpace(installationID))
	if !ValidInstallationID(want) {
		return time.Time{}, time.Time{}, errIdentityInvalid()
	}
	if licInstall != want {
		return time.Time{}, time.Time{}, errMismatch()
	}
	product := strings.TrimSpace(p.ProductCode)
	if !supportedProduct(product) {
		return time.Time{}, time.Time{}, errUnknownProduct()
	}
	if expected := strings.TrimSpace(expectedProduct); expected != "" && expected != product {
		return time.Time{}, time.Time{}, errWrongProduct()
	}
	issuedAt, err = parseLicenseTime(p.IssuedAt)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	expiresAt, err = parseLicenseTime(p.ExpiresAt)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !expiresAt.After(issuedAt) {
		return time.Time{}, time.Time{}, errInvalid()
	}
	now = now.UTC()
	if now.Before(issuedAt) {
		return time.Time{}, time.Time{}, errInvalid()
	}
	return issuedAt, expiresAt, nil
}
