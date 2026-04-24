package cli_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newLocalTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		if isLocalListenerDenied(err) {
			t.Skipf("local HTTP test server skipped: sandbox denied loopback listener creation: %v", err)
		}
		t.Fatalf("failed to start local test server: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func isLocalListenerDenied(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied")
}
