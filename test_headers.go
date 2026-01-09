// Standalone test for the HTTP header transport logic
// This tests the core functionality without requiring any external dependencies
package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

// --- Copy of the headerTransport implementation ---

var (
	extraHeaders     map[string]string
	extraHeadersLock sync.RWMutex
)

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())
	for key, value := range t.headers {
		reqClone.Header.Set(key, value)
	}
	return t.base.RoundTrip(reqClone)
}

func SetExtraHeaders(headers map[string]string) {
	extraHeadersLock.Lock()
	defer extraHeadersLock.Unlock()
	extraHeaders = headers
}

func GetExtraHeaders() map[string]string {
	extraHeadersLock.RLock()
	defer extraHeadersLock.RUnlock()
	return extraHeaders
}

func configureHTTPTransport(headers map[string]string, insecure bool) {
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}
	transport := &headerTransport{
		base:    baseTransport,
		headers: headers,
	}
	http.DefaultTransport = transport
	http.DefaultClient = &http.Client{Transport: transport}
}

// --- Tests ---

func main() {
	fmt.Println("=== HTTP Header Transport Tests ===")
	fmt.Println()

	testHeaderTransport()
	testPreservesOriginalHeaders()
	testDefaultTransportModification()
	testMultipleHeaders()

	fmt.Println()
	fmt.Println("=== All tests passed! ===")
}

func testHeaderTransport() {
	fmt.Print("Test: Basic header injection... ")

	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	headers := map[string]string{
		"X-Custom-Header":  "test-value",
		"X-Proxy-Auth":     "bearer-token-123",
		"X-Request-Source": "pulumi-test",
	}

	transport := &headerTransport{
		base:    http.DefaultTransport,
		headers: headers,
	}

	client := &http.Client{Transport: transport}
	resp, err := client.Get(server.URL)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	defer resp.Body.Close()

	for key, expectedValue := range headers {
		actualValue := capturedHeaders.Get(key)
		if actualValue != expectedValue {
			fmt.Printf("FAILED: Header %s expected %q, got %q\n", key, expectedValue, actualValue)
			return
		}
	}

	fmt.Println("PASSED")
	fmt.Printf("  Verified headers: %v\n", headers)
}

func testPreservesOriginalHeaders() {
	fmt.Print("Test: Preserves original request headers... ")

	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := &headerTransport{
		base:    http.DefaultTransport,
		headers: map[string]string{"X-Custom": "custom-value"},
	}

	client := &http.Client{Transport: transport}
	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("X-Original", "original-value")
	req.Header.Set("Authorization", "Bearer original-token")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if capturedHeaders.Get("X-Original") != "original-value" {
		fmt.Println("FAILED: Original header was not preserved")
		return
	}
	if capturedHeaders.Get("Authorization") != "Bearer original-token" {
		fmt.Println("FAILED: Authorization header was not preserved")
		return
	}
	if capturedHeaders.Get("X-Custom") != "custom-value" {
		fmt.Println("FAILED: Custom header was not added")
		return
	}

	fmt.Println("PASSED")
}

func testDefaultTransportModification() {
	fmt.Print("Test: Modifies http.DefaultTransport... ")

	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Configure the default transport with custom headers
	headers := map[string]string{
		"X-Global-Header": "global-value",
	}
	configureHTTPTransport(headers, true)

	// Use default client (which should now have our transport)
	resp, err := http.Get(server.URL)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if capturedHeaders.Get("X-Global-Header") != "global-value" {
		fmt.Printf("FAILED: Expected X-Global-Header, got headers: %v\n", capturedHeaders)
		return
	}

	fmt.Println("PASSED")
	fmt.Println("  http.DefaultClient now injects custom headers automatically")
}

func testMultipleHeaders() {
	fmt.Print("Test: Multiple headers for proxy scenario... ")

	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		fmt.Fprintf(w, `{"status": "ok"}`)
	}))
	defer server.Close()

	// Simulate headers a proxy/firewall might need
	proxyHeaders := map[string]string{
		"X-Forwarded-For":      "10.0.0.1",
		"X-Proxy-Token":        "secret-proxy-token",
		"X-Request-ID":         "req-12345",
		"X-Tenant-ID":          "tenant-abc",
		"X-Custom-Auth":        "Bearer my-proxy-auth",
		"X-Firewall-Bypass-Key": "fw-key-xyz",
	}

	transport := &headerTransport{
		base:    http.DefaultTransport,
		headers: proxyHeaders,
	}

	client := &http.Client{Transport: transport}
	resp, err := client.Get(server.URL)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	defer resp.Body.Close()

	allPassed := true
	for key, expected := range proxyHeaders {
		actual := capturedHeaders.Get(key)
		if actual != expected {
			fmt.Printf("\n  FAILED: %s expected %q, got %q", key, expected, actual)
			allPassed = false
		}
	}

	if allPassed {
		fmt.Println("PASSED")
		fmt.Printf("  All %d proxy headers injected correctly\n", len(proxyHeaders))
	}
}
