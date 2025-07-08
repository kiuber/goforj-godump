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

// HttpDebugTransport wraps a http.RoundTripper to optionally log requests and responses.
type HttpDebugTransport struct {
	Transport    http.RoundTripper
	debugEnabled bool
	dumper       *Dumper
}

// NewHttpDebugTransport creates a HttpDebugTransport with debug flag cached from env.
func NewHttpDebugTransport(inner http.RoundTripper) *HttpDebugTransport {
	return &HttpDebugTransport{
		Transport:    inner,
		debugEnabled: os.Getenv("HTTP_DEBUG") != "",
		dumper:       NewDumper(),
	}
}

// SetDebug allows toggling debug logging at runtime.
func (t *HttpDebugTransport) SetDebug(enabled bool) {
	t.debugEnabled = enabled
}

// Dumper returns the Dumper instance used for logging.
func (t *HttpDebugTransport) Dumper() *Dumper {
	return t.dumper
}

// RoundTrip implements the http.RoundTripper interface.
func (t *HttpDebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.debugEnabled {
		start := time.Now()

		// Dump Request
		reqDump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			reqPayload := parseHTTPDump("Request", string(reqDump))

			// Perform request
			resp, err := t.Transport.RoundTrip(req)
			duration := time.Since(start)
			if err != nil {
				return resp, err
			}

			// Dump Response
			respDump, err := httputil.DumpResponse(resp, true)
			if err == nil {
				respPayload := parseHTTPDump("Response", string(respDump))

				// Combine and dump
				transaction := map[string]any{
					"Transaction": map[string]any{
						"Request":  reqPayload,
						"Response": respPayload,
						"Duration": fmt.Sprintf("%v", duration),
					},
				}
				t.dumper.Dump(transaction)
			}
			return resp, nil
		}
	}

	// No debug: straight pass-through
	return t.Transport.RoundTrip(req)
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
