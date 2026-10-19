package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testClientID     = "test-client-id"
	testClientSecret = "test-client-secret"
	testRedirectURL  = "https://yourapp.com/auth/callback"
	testNonce        = "test-nonce"
	testCode         = "test-authorization-code"
)

// fakeOIDCProvider serves the discovery document, JWKS and token endpoint a real
// provider would expose, so the client can be tested against the actual OIDC library.
type fakeOIDCProvider struct {
	t      *testing.T
	server *httptest.Server
	key    *rsa.PrivateKey

	mu            sync.Mutex
	exchangeCount int
	rejectTokens  bool
	claims        map[string]any
}

func newFakeOIDCProvider(t *testing.T) *fakeOIDCProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}

	provider := &fakeOIDCProvider{t: t, key: key}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", provider.discovery)
	mux.HandleFunc("/jwks", provider.jwks)
	mux.HandleFunc("/token", provider.token)

	provider.server = httptest.NewServer(mux)
	t.Cleanup(provider.server.Close)

	return provider
}

func (p *fakeOIDCProvider) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                p.server.URL,
		"authorization_endpoint":                p.server.URL + "/authorize",
		"token_endpoint":                        p.server.URL + "/token",
		"jwks_uri":                              p.server.URL + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (p *fakeOIDCProvider) jwks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": "test-key",
			"n":   base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.E)).Bytes()),
		}},
	})
}

func (p *fakeOIDCProvider) token(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.exchangeCount++
	reject := p.rejectTokens
	claims := p.claims
	p.mu.Unlock()

	if reject {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":             "invalid_grant",
			"error_description": "authorization code expired",
		})
		return
	}

	if err := r.ParseForm(); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	if got := r.Form.Get("code"); got != testCode {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "unknown code"})
		return
	}

	if r.Form.Get("code_verifier") == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "error_description": "missing code_verifier"})
		return
	}

	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		clientID = r.Form.Get("client_id")
		clientSecret = r.Form.Get("client_secret")
	}
	if clientID != testClientID || clientSecret != testClientSecret {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "invalid_client"})
		return
	}

	idToken, err := signIDToken(p.key, mergeClaims(map[string]any{
		"iss":   p.server.URL,
		"aud":   testClientID,
		"sub":   "subject-123",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
		"nonce": testNonce,
	}, claims))
	if err != nil {
		p.t.Errorf("failed to sign id token: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"access_token": "access-token",
		"token_type":   "Bearer",
		"expires_in":   300,
		"id_token":     idToken,
	})
}

func (p *fakeOIDCProvider) exchangeCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exchangeCount
}

func newTestClient(t *testing.T, provider *fakeOIDCProvider) *OIDCClient {
	t.Helper()

	client, err := NewOIDCClient(
		context.Background(),
		provider.server.URL,
		testClientID,
		testClientSecret,
		testRedirectURL,
		[]string{"openid", "profile", "email"},
	)
	if err != nil {
		t.Fatalf("NewOIDCClient() returned unexpected error: %v", err)
	}

	return client
}

func TestAuthCodeURLIncludesStateNonceAndPKCE(t *testing.T) {
	client := newTestClient(t, newFakeOIDCProvider(t))
	verifier := NewCodeVerifier()

	authURL, err := url.Parse(client.AuthCodeURL("test-state", testNonce, verifier))
	if err != nil {
		t.Fatalf("failed to parse auth url: %v", err)
	}

	query := authURL.Query()
	checks := map[string]string{
		"client_id":             testClientID,
		"redirect_uri":          testRedirectURL,
		"response_type":         "code",
		"state":                 "test-state",
		"nonce":                 testNonce,
		"code_challenge_method": "S256",
	}
	for name, want := range checks {
		if got := query.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	sum := sha256.Sum256([]byte(verifier))
	if got, want := query.Get("code_challenge"), base64.RawURLEncoding.EncodeToString(sum[:]); got != want {
		t.Errorf("code_challenge = %q, want %q", got, want)
	}

	if !strings.Contains(query.Get("scope"), "openid") {
		t.Errorf("scope = %q, want it to contain openid", query.Get("scope"))
	}
}

func TestExchangeVerifiesIDTokenAndExtractsUser(t *testing.T) {
	provider := newFakeOIDCProvider(t)
	provider.claims = map[string]any{
		"preferred_username": "gburdell3",
		"email":              "george.burdell@gatech.edu",
		"name":               "George P. Burdell",
	}
	client := newTestClient(t, provider)

	user, err := client.Exchange(context.Background(), testCode, NewCodeVerifier(), testNonce)
	if err != nil {
		t.Fatalf("Exchange() returned unexpected error: %v", err)
	}

	if user.Username != "gburdell3" {
		t.Errorf("Username = %q, want %q", user.Username, "gburdell3")
	}
	if got := user.Attributes["email"]; got != "george.burdell@gatech.edu" {
		t.Errorf("email attribute = %q, want %q", got, "george.burdell@gatech.edu")
	}
	if got := user.Attributes["name"]; got != "George P. Burdell" {
		t.Errorf("name attribute = %q, want %q", got, "George P. Burdell")
	}
	if _, ok := user.Attributes["nonce"]; ok {
		t.Error("nonce should not be exposed as a user attribute")
	}
	if provider.exchangeCalls() != 1 {
		t.Errorf("token endpoint called %d times, want 1", provider.exchangeCalls())
	}
}

func TestExchangeRejectsWrongNonce(t *testing.T) {
	client := newTestClient(t, newFakeOIDCProvider(t))

	_, err := client.Exchange(context.Background(), testCode, NewCodeVerifier(), "a-different-nonce")
	if err == nil {
		t.Fatal("Exchange() = nil error, want a nonce mismatch")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != "invalid_nonce" {
		t.Errorf("Exchange() error = %v, want a ValidationError with code invalid_nonce", err)
	}
}

func TestExchangeReturnsValidationErrorForProviderRejection(t *testing.T) {
	provider := newFakeOIDCProvider(t)
	provider.rejectTokens = true
	client := newTestClient(t, provider)

	_, err := client.Exchange(context.Background(), testCode, NewCodeVerifier(), testNonce)
	if err == nil {
		t.Fatal("Exchange() = nil error, want an error")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Exchange() error = %v, want a ValidationError", err)
	}
	if validationErr.Code != "invalid_grant" {
		t.Errorf("ValidationError.Code = %q, want %q", validationErr.Code, "invalid_grant")
	}
	if validationErr.Message != "authorization code expired" {
		t.Errorf("ValidationError.Message = %q", validationErr.Message)
	}
}

func TestExchangeEmptyCodeDoesNotCallProvider(t *testing.T) {
	provider := newFakeOIDCProvider(t)
	client := newTestClient(t, provider)

	_, err := client.Exchange(context.Background(), "  ", NewCodeVerifier(), testNonce)
	if err == nil {
		t.Fatal("Exchange() = nil error, want an error for an empty code")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != "invalid_request" {
		t.Errorf("Exchange() error = %v, want a ValidationError with code invalid_request", err)
	}
	if provider.exchangeCalls() != 0 {
		t.Errorf("token endpoint called %d times, want 0", provider.exchangeCalls())
	}
}

func TestUsernameFromClaimsFallsBackThroughAvailableClaims(t *testing.T) {
	tests := []struct {
		name    string
		claims  map[string]any
		subject string
		want    string
	}{
		{
			name:    "username claim wins",
			claims:  map[string]any{"username": "gburdell3", "preferred_username": "other"},
			subject: "sub",
			want:    "gburdell3",
		},
		{
			name:    "email local part",
			claims:  map[string]any{"preferred_username": "george.burdell@gatech.edu"},
			subject: "sub",
			want:    "george.burdell",
		},
		{
			name:    "subject fallback",
			claims:  map[string]any{},
			subject: "subject-123",
			want:    "subject-123",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usernameFromClaims(test.claims, test.subject); got != test.want {
				t.Errorf("usernameFromClaims() = %q, want %q", got, test.want)
			}
		})
	}
}

func mergeClaims(base, override map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(override))
	for name, value := range base {
		merged[name] = value
	}
	for name, value := range override {
		merged[name] = value
	}
	return merged
}

func signIDToken(key *rsa.PrivateKey, claims map[string]any) (string, error) {
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": "test-key"}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func writeJSON(w http.ResponseWriter, body any) {
	writeJSONStatus(w, http.StatusOK, body)
}

func writeJSONStatus(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
