package dcache

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {

	c := caddy.NewTestController("dns", `dcache`)
	if err := setup(c); err == nil {
		t.Fatalf("Expected errors, but got no error")
	}

	c = caddy.NewTestController("dns", `dcache 127.0.0.1`)
	if err := setup(c); err == nil {
		t.Fatalf("Expected errors, but got no error")
	}

	c = caddy.NewTestController("dns", `dcache 127.0.0.1:6379`)
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %v", err)
	}
}
