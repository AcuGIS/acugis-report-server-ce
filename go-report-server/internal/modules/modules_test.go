package modules

import "testing"

func TestCoreAlwaysRegistered(t *testing.T) {
	if !Has(IDCore) {
		t.Fatal("core module must be registered")
	}
}
