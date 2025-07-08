package godump

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHttpDebugTransport_WithDebugEnabled(t *testing.T) {
	var buf bytes.Buffer

	// Enable HTTP_DEBUG
	_ = os.Setenv("HTTP_DEBUG", "1")
	defer os.Unsetenv("HTTP_DEBUG")

	transport := NewHttpDebugTransport(http.DefaultTransport)
	transport.Dumper().writer = &buf // Redirect Dumper to buffer

	client := &http.Client{Transport: transport}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "TestValue")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Captured dump:\n%s", output)

	if !strings.Contains(output, "Transaction =>") {
		t.Errorf("expected 'Transaction =>' in dump, got none")
	}
	if !strings.Contains(output, "Request =>") {
		t.Errorf("expected 'Request =>' in dump, got none")
	}
	if !strings.Contains(output, "Response =>") {
		t.Errorf("expected 'Response =>' in dump, got none")
	}
	if !strings.Contains(output, `"success":true`) {
		t.Errorf("expected JSON body in dump")
	}
}

func TestHttpDebugTransport_WithDebugDisabled(t *testing.T) {
	var buf bytes.Buffer

	// Disable HTTP_DEBUG
	_ = os.Unsetenv("HTTP_DEBUG")

	transport := NewHttpDebugTransport(http.DefaultTransport)
	transport.Dumper().writer = &buf // Redirect Dumper to buffer

	client := &http.Client{Transport: transport}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Captured dump:\n%s", output)

	if strings.Contains(output, "Transaction =>") {
		t.Errorf("did not expect 'Transaction =>' in dump when debug disabled")
	}
}

func TestHttpDebugTransport_SetDebugToggle(t *testing.T) {
	var buf bytes.Buffer

	transport := NewHttpDebugTransport(http.DefaultTransport)
	transport.Dumper().writer = &buf // Redirect Dumper to buffer

	client := &http.Client{Transport: transport}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "DynamicTest")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Debug initially disabled: no dump
	transport.SetDebug(false)

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Dump with debug disabled:\n%s", output)
	if strings.Contains(output, "Transaction =>") {
		t.Errorf("did not expect dump output when debug disabled")
	}

	// Enable debug
	transport.SetDebug(true)
	buf.Reset()

	req, _ = http.NewRequest("GET", server.URL, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	fmt.Println(buf.String())

	output = stripANSI(buf.String())
	t.Logf("Dump with debug enabled:\n%s", output)
	if !strings.Contains(output, "Transaction =>") {
		t.Errorf("expected dump output after enabling debug")
	}

	// Disable debug again
	transport.SetDebug(false)
	buf.Reset()

	req, _ = http.NewRequest("GET", server.URL, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	output = stripANSI(buf.String())
	t.Logf("Dump after re-disabling debug:\n%s", output)
	if strings.Contains(output, "Transaction =>") {
		t.Errorf("did not expect dump output after re-disabling debug")
	}
}

// roundTripFunc allows mocking http.RoundTripper in tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHttpDebugTransport_RoundTripError(t *testing.T) {
	var buf bytes.Buffer

	// Create transport and redirect Dumper
	transport := NewHttpDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("simulated network error")
	}))
	transport.Dumper().writer = &buf
	transport.SetDebug(true) // Force debug on

	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", "http://example.invalid", nil)
	_, err := client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "simulated network error") {
		t.Fatalf("expected simulated network error, got: %v", err)
	}

	output := stripANSI(buf.String())
	t.Logf("Captured dump (error case):\n%s", output)

	// Should NOT contain Transaction because response was nil
	if strings.Contains(output, "Transaction =>") {
		t.Errorf("did not expect Transaction block when RoundTrip failed")
	}
}
