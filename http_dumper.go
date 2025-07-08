package godump

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"sort"
	"strings"
	"time"
)

// HTTPDebugTransport wraps a http.RoundTripper to optionally log requests and responses.
type HTTPDebugTransport struct {
	Transport    http.RoundTripper
	debugEnabled bool
	dumper       *Dumper
}

// NewHTTPDebugTransport creates a HTTPDebugTransport with debug flag cached from env.
func NewHTTPDebugTransport(inner http.RoundTripper) *HTTPDebugTransport {
	return &HTTPDebugTransport{
		Transport:    inner,
		debugEnabled: os.Getenv("HTTP_DEBUG") != "",
		dumper:       NewDumper(WithSkipStackFrames(4)),
	}
}

// SetDebug allows toggling debug logging at runtime.
func (t *HTTPDebugTransport) SetDebug(enabled bool) {
	t.debugEnabled = enabled
}

// Dumper returns the Dumper instance used for logging.
func (t *HTTPDebugTransport) Dumper() *Dumper {
	return t.dumper
}

// RoundTrip implements the http.RoundTripper interface.
func (t *HTTPDebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.debugEnabled {
		resp, err := t.Transport.RoundTrip(req)
		if err != nil {
			return resp, fmt.Errorf("HTTPDebugTransport: pass-through round trip failed: %w", err)
		}
		return resp, nil
	}

	start := time.Now()

	// Dump Request
	reqDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return nil, fmt.Errorf("HTTPDebugTransport: failed to dump request: %w", err)
	}
	request := parseHTTPDump("Request", string(reqDump))

	// Perform request
	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("HTTPDebugTransport: round trip failed: %w", err)
	}
	duration := time.Since(start)

	// Dump Response
	resDump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return resp, nil // Still return resp even if dump fails
	}
	response := parseHTTPDump("Response", string(resDump))

	// Combine and dump
	transaction := map[string]any{
		"Transaction": map[string]any{
			"Request":  request,
			"Response": response,
			"Duration": duration.String(),
		},
	}
	t.dumper.Dump(transaction)

	return resp, nil
}

// parseHTTPDump parses the raw HTTP dump into a structured map.
func parseHTTPDump(label, raw string) map[string]any {
	lines := strings.Split(raw, "\n")
	payload := make(map[string]any)
	headers := make(map[string]string)
	inBody := false
	var bodyBuilder strings.Builder

	for i, line := range lines {
		line = strings.TrimRight(line, "\r\n")

		if i == 0 {
			if label == "Request" {
				payload["Request-Line"] = line
			} else {
				payload["Status"] = line
			}
			continue
		}

		if inBody {
			bodyBuilder.WriteString(line + "\n")
			continue
		}

		if line == "" {
			inBody = true
			continue
		}

		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			headers[key] = value
		}
	}

	// Alphabetize headers
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		payload[k] = headers[k]
	}

	// Add body as raw
	body := strings.TrimSpace(bodyBuilder.String())
	if body != "" {
		payload["Body"] = body
	}

	return payload
}
