package main

import (
	"net/http"
	"testing"
)

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	server := newHTTPServer(":8000", http.NewServeMux())

	if server.ReadHeaderTimeout == 0 {
		t.Fatal("expected ReadHeaderTimeout to be configured")
	}
	if server.ReadTimeout == 0 {
		t.Fatal("expected ReadTimeout to be configured")
	}
	if server.WriteTimeout == 0 {
		t.Fatal("expected WriteTimeout to be configured")
	}
	if server.IdleTimeout == 0 {
		t.Fatal("expected IdleTimeout to be configured")
	}
}
