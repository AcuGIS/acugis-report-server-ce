package license

import "crypto/ed25519"

// productionPublicKeyHex is the AcuGIS production verification key.
//
// Provenance: exported on the license host with
//
//	acugis-license-server public-key --key /etc/acugis-license/signing.key
//
// That command prints only the Ed25519 public key. The private signing key
// was not read into this repository. Tests use separately generated keys.
// Packaging may override this value with -ldflags
// -X github.com/acugis/acugis-report-server-go/internal/license.productionPublicKeyHex=...
// from ACUGIS_LICENSE_PUBLIC_KEY_HEX. The installed binary does not fetch
// the key from the license server.
var productionPublicKeyHex = "ff2436eabcc7e2d5597eacdb9a786730ef168420e040ccfd83185fec2200ed20"

// ProductionPublicKey returns the embedded AcuGIS verification key.
func ProductionPublicKey() (ed25519.PublicKey, error) {
	return ParsePublicKeyHex(productionPublicKeyHex)
}
