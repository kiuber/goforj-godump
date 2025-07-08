package godump

import (
	"bytes"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

var ErrSimulatedNetwork = errors.New("simulated network error")

func TestHTTPDebugTransport_WithDebugEnabled(t *testing.T) {
	var buf bytes.Buffer

	// Enable HTTP_DEBUG environment variable
	t.Setenv("HTTP_DEBUG", "1")

	// Create a new HTTPDebugTransport with debug enabled
	tp := NewHTTPDebugTransport(http.DefaultTransport)
	tp.Dumper().writer = &buf
	tp.SetDebug(true)

	// Create an HTTP client with the debug transport
	client := &http.Client{Transport: tp}

	// Create a test server that responds with a custom header and JSON body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "TestValue")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"success":true}`))
		assert.NoError(t, err, "failed to write response")
	}))
	defer server.Close()

	//nolint:noctx // no context needed for this unit test: synthetic request
	req, err := http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err, "expected no error from client.Do")
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Captured dump:\n%s", output)

	assert.Contains(t, output, "Transaction =>", "expected 'Transaction =>' in dump")
	assert.Contains(t, output, "Request =>", "expected 'Request =>' in dump")
	assert.Contains(t, output, "Response =>", "expected 'Response =>' in dump")
	assert.Contains(t, output, `"success":true`, "expected JSON body in dump")
}

func TestHTTPDebugTransport_WithDebugDisabled(t *testing.T) {
	var buf bytes.Buffer

	tp := NewHTTPDebugTransport(http.DefaultTransport)
	tp.Dumper().writer = &buf

	client := &http.Client{Transport: tp}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	//nolint:noctx // no context needed for this unit test: synthetic request
	req, _ := http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Captured dump:\n%s", output)

	assert.NotContains(t, output, "Transaction =>", "did not expect 'Transaction =>' in dump when debug disabled")
}

func TestHTTPDebugTransport_RoundTripError(t *testing.T) {
	var buf bytes.Buffer

	tp := NewHTTPDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, ErrSimulatedNetwork
	}))
	tp.Dumper().writer = &buf
	tp.SetDebug(true)

	client := &http.Client{Transport: tp}

	//nolint:noctx // no context needed for this unit test: synthetic request
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid", http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.Error(t, err, "expected simulated network error, got none")
	require.Contains(t, err.Error(), "simulated network error", "expected error to contain 'simulated network error'")

	if resp != nil {
		defer resp.Body.Close()
	}

	output := stripANSI(buf.String())
	t.Logf("Captured dump (error case):\n%s", output)

	assert.NotContains(t, output, "Transaction =>", "did not expect Transaction block when RoundTrip failed")
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

	//nolint:noctx // no context needed for this unit test: synthetic request
	req, err := http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	output := stripANSI(buf.String())
	t.Logf("Dump with debug disabled:\n%s", output)
	assert.NotContains(t, output, "Transaction =>") // Should not be present

	// Enable debug
	transport.SetDebug(true)
	buf.Reset()

	//nolint:noctx // no context needed for this unit test: synthetic request
	req, err = http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	output = stripANSI(buf.String())
	t.Logf("Dump with debug enabled:\n%s", output)
	assert.Contains(t, output, "Transaction =>")
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

	// Create a client with the debug transport
	client := &http.Client{Transport: transport}

	req, err := http.NewRequest(http.MethodGet, "http://example.invalid", http.NoBody)
	require.NoError(t, err)

	// Simulate a pass-through failure
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPDebugTransport: pass-through round trip failed")
	require.ErrorIs(t, err, ErrSimulatedTransportFailure)

	require.Nil(t, resp)
}

func TestHTTPDebugTransport_RequestDumpFailure(t *testing.T) {
	tp := NewHTTPDebugTransport(http.DefaultTransport)
	tp.SetDebug(true)

	client := &http.Client{Transport: tp}

	// Malformed request: URL exists but has no Scheme/Host
	req := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{},
		Header: http.Header{},
	}

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPDebugTransport: failed to dump request")
}

type errorBody struct{}

var errSimulatedBodyReadFailure = errors.New("simulated body read failure")

func (errorBody) Read(p []byte) (int, error) { return 0, errSimulatedBodyReadFailure }
func (errorBody) Close() error               { return nil }

func TestHTTPDebugTransport_ResponseDumpFailure(t *testing.T) {
	transport := NewHTTPDebugTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(errorBody{}),
		}, nil
	}))
	transport.SetDebug(true)

	client := &http.Client{Transport: transport}

	//nolint:noctx // no context needed for this unit test: synthetic request
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid", http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPDebugTransport: failed to dump response")
	require.ErrorIs(t, err, errSimulatedBodyReadFailure)
	require.Nil(t, resp)
}
