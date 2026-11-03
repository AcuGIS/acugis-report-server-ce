//go:build !unix

package fileown

// Apply is a no-op on non-Unix platforms (ownership is a Tomcat/Linux concern).
func Apply(path, owner, group string) error {
	return nil
}
