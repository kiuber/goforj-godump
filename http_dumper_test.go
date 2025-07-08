package godump

import (
	"bytes"
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

var ErrSimulatedNetwork = errors.New("simulated network error")

func TestHTTPDebugTransport_WithDebugEnabled(t *testing.T) {
	var buf bytes.Buffer

	_ = os.Setenv("HTTP_DEBUG", "1")
	defer os.Unsetenv("HTTP_DEBUG")

	transport := NewHTTPDebugTransport(http.DefaultTransport)
	transport.Dumper().writer = &buf
	transport.SetDebug(true)

	client := &http.Client{Transport: transport}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "TestValue")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"success":true}`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Captured dump:\n%s", output)

	if !strings.Contains(output, "Transaction =>") {
		t.Error("expected 'Transaction =>' in dump, got none")
	}
	if !strings.Contains(output, "Request =>") {
		t.Error("expected 'Request =>' in dump, got none")
	}
	if !strings.Contains(output, "Response =>") {
		t.Error("expected 'Response =>' in dump, got none")
	}
	if !strings.Contains(output, `"success":true`) {
		t.Error("expected JSON body in dump")
	}
}

func TestHTTPDebugTransport_WithDebugDisabled(t *testing.T) {
	var buf bytes.Buffer

	_ = os.Unsetenv("HTTP_DEBUG")

	transport := NewHTTPDebugTransport(http.DefaultTransport)
	transport.Dumper().writer = &buf

	client := &http.Client{Transport: transport}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Captured dump:\n%s", output)

	if strings.Contains(output, "Transaction =>") {
		t.Error("did not expect 'Transaction =>' in dump when debug disabled")
	}
}

func TestHTTPDebugTransport_RoundTripError(t *testing.T) {
	var buf bytes.Buffer

	transport := NewHTTPDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, ErrSimulatedNetwork
	}))
	transport.Dumper().writer = &buf
	transport.SetDebug(true)

	client := &http.Client{Transport: transport}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid", http.NoBody)
	_, err := client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "simulated network error") {
		t.Fatalf("expected simulated network error, got: %v", err)
	}

	output := stripANSI(buf.String())
	t.Logf("Captured dump (error case):\n%s", output)

	if strings.Contains(output, "Transaction =>") {
		t.Error("did not expect Transaction block when RoundTrip failed")
	}
}

func TestHTTPDebugTransport_SetDebugToggle(t *testing.T) {
	var buf bytes.Buffer

	transport := NewHTTPDebugTransport(http.DefaultTransport)
	transport.Dumper().writer = &buf

	client := &http.Client{Transport: transport}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "DynamicTest")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Debug disabled
	transport.SetDebug(false)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Dump with debug disabled:\n%s", output)
	if strings.Contains(output, "Transaction =>") {
		t.Error("did not expect dump output when debug disabled")
	}

	// Enable debug
	transport.SetDebug(true)
	buf.Reset()

	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output = stripANSI(buf.String())
	t.Logf("Dump with debug enabled:\n%s", output)
	if !strings.Contains(output, "Transaction =>") {
		t.Error("expected dump output after enabling debug")
	}
}

// roundTripFunc lets us use a function as a RoundTripper in tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

var ErrSimulatedTransportFailure = errors.New("simulated transport failure")

func TestHTTPDebugTransport_PassThroughRoundTripError(t *testing.T) {
	transport := NewHTTPDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, ErrSimulatedTransportFailure
	}))
	transport.SetDebug(false)

	client := &http.Client{Transport: transport}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid", http.NoBody)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPDebugTransport: pass-through round trip failed")
	assert.ErrorIs(t, err, ErrSimulatedTransportFailure)
}
