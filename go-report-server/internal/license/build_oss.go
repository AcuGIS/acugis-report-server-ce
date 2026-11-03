//go:build oss

package license

// packageCapable is false in the OSS binary. A Professional ACL1 on disk
// does not enable Professional features.
const packageCapable = false
