// Package shim provides a wrapper around the upstream terraform-provider-rancher2
// that adds support for custom HTTP headers.
package shim

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/rancher/terraform-provider-rancher2/rancher2"
)

var (
	// extraHeaders stores the custom headers to be added to all requests
	extraHeaders     map[string]string
	extraHeadersLock sync.RWMutex
)

// headerTransport wraps an http.RoundTripper to add custom headers
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid modifying the original
	reqClone := req.Clone(req.Context())
	for key, value := range t.headers {
		reqClone.Header.Set(key, value)
	}
	return t.base.RoundTrip(reqClone)
}

// SetExtraHeaders sets the custom headers to be added to all requests
func SetExtraHeaders(headers map[string]string) {
	extraHeadersLock.Lock()
	defer extraHeadersLock.Unlock()
	extraHeaders = headers
}

// GetExtraHeaders returns the current custom headers
func GetExtraHeaders() map[string]string {
	extraHeadersLock.RLock()
	defer extraHeadersLock.RUnlock()
	return extraHeaders
}

// Provider returns the upstream rancher2 provider with extra_headers support
func Provider() *schema.Provider {
	// Get the upstream provider
	upstream := rancher2.Provider().(*schema.Provider)

	// Add extra_headers schema field
	upstream.Schema["extra_headers"] = &schema.Schema{
		Type:        schema.TypeMap,
		Optional:    true,
		Description: "Extra HTTP headers to include in all API requests to the Rancher server. Useful for proxies or firewalls. Can also be set via RANCHER_EXTRA_HEADERS environment variable as a JSON object.",
		DefaultFunc: schema.EnvDefaultFunc("RANCHER_EXTRA_HEADERS", nil),
		Elem: &schema.Schema{
			Type: schema.TypeString,
		},
	}

	// Wrap the original ConfigureFunc to capture headers and set up custom transport
	originalConfigure := upstream.ConfigureFunc
	upstream.ConfigureFunc = func(d *schema.ResourceData) (interface{}, error) {
		// Parse extra headers from config or environment
		headers := make(map[string]string)

		if v, ok := d.GetOk("extra_headers"); ok {
			for key, val := range v.(map[string]interface{}) {
				headers[key] = val.(string)
			}
		}

		// Also check environment variable as JSON
		if envHeaders := os.Getenv("RANCHER_EXTRA_HEADERS"); envHeaders != "" && len(headers) == 0 {
			var envMap map[string]string
			if err := json.Unmarshal([]byte(envHeaders), &envMap); err == nil {
				headers = envMap
			}
		}

		// Store headers for later use
		if len(headers) > 0 {
			SetExtraHeaders(headers)
			// Modify default HTTP transport to include headers
			configureHTTPTransport(headers, d)
		}

		// Call original configure
		return originalConfigure(d)
	}

	return upstream
}

// configureHTTPTransport sets up the default HTTP client with custom headers
func configureHTTPTransport(headers map[string]string, d *schema.ResourceData) {
	// Get insecure setting
	insecure := false
	if v, ok := d.GetOk("insecure"); ok {
		insecure = v.(bool)
	}

	// Create base transport
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}

	// Handle CA certs if provided
	if v, ok := d.GetOk("ca_certs"); ok && v.(string) != "" {
		// The rancher2 provider handles CA certs internally
		// We just need to set up the header transport
	}

	// Create header transport wrapper
	transport := &headerTransport{
		base:    baseTransport,
		headers: headers,
	}

	// Set as default transport
	http.DefaultTransport = transport
	http.DefaultClient = &http.Client{Transport: transport}
}

// GetTransportWithHeaders returns an http.RoundTripper that adds the configured headers
func GetTransportWithHeaders(base http.RoundTripper) http.RoundTripper {
	headers := GetExtraHeaders()
	if len(headers) == 0 {
		return base
	}
	return &headerTransport{
		base:    base,
		headers: headers,
	}
}

// ParseHeadersString parses a header string in format "Key1: Value1, Key2: Value2"
func ParseHeadersString(s string) map[string]string {
	headers := make(map[string]string)
	if s == "" {
		return headers
	}

	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				headers[key] = value
			}
		}
	}
	return headers
}
