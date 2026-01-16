package client

import "net/http"

// AuthProvider applies authentication to an HTTP request.
// Implementations can add headers, query params, or signatures
// depending on the target API requirements.
type AuthProvider interface {
	Apply(req *http.Request) error
}

// APIKeySecretHeaderAuth sets API key/secret via simple headers.
// Header names are configurable to avoid hard-coding provider specifics.
type APIKeySecretHeaderAuth struct {
	KeyHeader    string
	SecretHeader string
	APIKey       string
	APISecret    string
}

func (a APIKeySecretHeaderAuth) Apply(req *http.Request) error {
	if a.APIKey != "" && a.KeyHeader != "" {
		req.Header.Set(a.KeyHeader, a.APIKey)
	}
	if a.APISecret != "" && a.SecretHeader != "" {
		req.Header.Set(a.SecretHeader, a.APISecret)
	}
	return nil
}
