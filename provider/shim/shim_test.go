package shim

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeaderTransport(t *testing.T) {
	// Create a test server that captures headers
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create header transport with custom headers
	headers := map[string]string{
		"X-Custom-Header":  "test-value",
		"X-Proxy-Auth":     "bearer-token-123",
		"X-Request-Source": "pulumi-test",
	}

	transport := &headerTransport{
		base:    http.DefaultTransport,
		headers: headers,
	}

	// Create client with our transport
	client := &http.Client{Transport: transport}

	// Make request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify headers were added
	for key, expectedValue := range headers {
		actualValue := capturedHeaders.Get(key)
		if actualValue != expectedValue {
			t.Errorf("Header %s: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	t.Logf("All custom headers were correctly injected!")
	t.Logf("Captured headers: %v", capturedHeaders)
}

func TestSetAndGetExtraHeaders(t *testing.T) {
	// Test setting and getting headers
	headers := map[string]string{
		"X-Test": "value1",
		"X-Auth": "value2",
	}

	SetExtraHeaders(headers)
	retrieved := GetExtraHeaders()

	if len(retrieved) != len(headers) {
		t.Errorf("Expected %d headers, got %d", len(headers), len(retrieved))
	}

	for key, expected := range headers {
		if retrieved[key] != expected {
			t.Errorf("Header %s: expected %q, got %q", key, expected, retrieved[key])
		}
	}
}

func TestParseHeadersString(t *testing.T) {
	tests := []struct {
		input    string
		expected map[string]string
	}{
		{
			input:    "X-Header1: value1, X-Header2: value2",
			expected: map[string]string{"X-Header1": "value1", "X-Header2": "value2"},
		},
		{
			input:    "Authorization: Bearer token123",
			expected: map[string]string{"Authorization": "Bearer token123"},
		},
		{
			input:    "",
			expected: map[string]string{},
		},
		{
			input:    "  X-Spaced  :  spaced value  ",
			expected: map[string]string{"X-Spaced": "spaced value"},
		},
	}

	for _, tc := range tests {
		result := ParseHeadersString(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("Input %q: expected %d headers, got %d", tc.input, len(tc.expected), len(result))
			continue
		}
		for key, expected := range tc.expected {
			if result[key] != expected {
				t.Errorf("Input %q, key %s: expected %q, got %q", tc.input, key, expected, result[key])
			}
		}
	}
}

func TestGetTransportWithHeaders(t *testing.T) {
	// Clear any existing headers
	SetExtraHeaders(nil)

	// Without headers, should return base transport
	base := http.DefaultTransport
	result := GetTransportWithHeaders(base)
	if result != base {
		t.Error("Expected base transport when no headers set")
	}

	// With headers, should return wrapped transport
	SetExtraHeaders(map[string]string{"X-Test": "value"})
	result = GetTransportWithHeaders(base)
	if result == base {
		t.Error("Expected wrapped transport when headers are set")
	}

	// Verify it's a headerTransport
	if _, ok := result.(*headerTransport); !ok {
		t.Error("Expected result to be *headerTransport")
	}
}

func TestHeaderTransportPreservesOriginalHeaders(t *testing.T) {
	// Create a test server that captures headers
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create header transport
	transport := &headerTransport{
		base:    http.DefaultTransport,
		headers: map[string]string{"X-Custom": "custom-value"},
	}

	client := &http.Client{Transport: transport}

	// Create request with existing headers
	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("X-Original", "original-value")
	req.Header.Set("Authorization", "Bearer original-token")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify original headers are preserved
	if capturedHeaders.Get("X-Original") != "original-value" {
		t.Error("Original header X-Original was not preserved")
	}
	if capturedHeaders.Get("Authorization") != "Bearer original-token" {
		t.Error("Original Authorization header was not preserved")
	}

	// Verify custom header was added
	if capturedHeaders.Get("X-Custom") != "custom-value" {
		t.Error("Custom header X-Custom was not added")
	}
}
