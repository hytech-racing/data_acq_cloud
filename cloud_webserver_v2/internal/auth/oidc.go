package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// ErrAuthenticationFailed is returned when the provider answered, but the answer could
// not be turned into an authenticated user.
var ErrAuthenticationFailed = errors.New("oidc authentication failed")

// ValidationError describes a login the provider definitively rejected. Unlike transport
// or discovery errors, repeating the same request will not change the outcome.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Message == "" {
		return "oidc validation failed: " + e.Code
	}
	return fmt.Sprintf("oidc validation failed: %s: %s", e.Code, e.Message)
}

// User is a person authenticated by the identity provider.
type User struct {
	Username   string            `json:"username"`
	Attributes map[string]string `json:"attributes"`
}

// OIDCClient implements the OpenID Connect authorization code flow for a single
// registered client. Discovery happens once, at construction time, so a misconfigured
// issuer or client fails the server at startup rather than on the first login attempt.
type OIDCClient struct {
	verifier     *oidc.IDTokenVerifier
	oauth2Config *oauth2.Config
	httpClient   *http.Client
}

func NewOIDCClient(ctx context.Context, issuerURL, clientID, clientSecret, redirectURL string, scopes []string) (*OIDCClient, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, httpClient), strings.TrimRight(issuerURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("failed to discover oidc provider at %s: %w", issuerURL, err)
	}

	return &OIDCClient{
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		oauth2Config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     provider.Endpoint(),
			// Without the openid scope the provider does not issue an id token.
			Scopes: parseScopes(strings.Join(scopes, " ")),
		},
		httpClient: httpClient,
	}, nil
}

// AuthCodeURL builds the provider login URL. The state binds the callback to this
// browser, the nonce binds the id token to this request, and the verifier enables PKCE.
func (c *OIDCClient) AuthCodeURL(state, nonce, codeVerifier string) string {
	return c.oauth2Config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(codeVerifier),
	)
}

// Exchange redeems a one-time authorization code and verifies the returned id token.
// The code must be exchanged immediately: it expires within seconds and can only be used
// once.
func (c *OIDCClient) Exchange(ctx context.Context, code, codeVerifier, expectedNonce string) (*User, error) {
	if strings.TrimSpace(code) == "" {
		return nil, &ValidationError{Code: "invalid_request", Message: "missing authorization code"}
	}

	ctx = oidc.ClientContext(ctx, c.httpClient)

	token, err := c.oauth2Config.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) {
			errorCode := retrieveErr.ErrorCode
			if errorCode == "" {
				errorCode = "invalid_grant"
			}
			return nil, &ValidationError{Code: errorCode, Message: retrieveErr.ErrorDescription}
		}
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, &ValidationError{Code: "invalid_response", Message: "token response did not include an id_token"}
	}

	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, &ValidationError{Code: "invalid_id_token", Message: err.Error()}
	}

	if expectedNonce != "" && idToken.Nonce != expectedNonce {
		return nil, &ValidationError{Code: "invalid_nonce", Message: "id token nonce did not match the login request"}
	}

	return userFromIDToken(idToken)
}

// NewCodeVerifier returns a PKCE code verifier.
func NewCodeVerifier() string {
	return oauth2.GenerateVerifier()
}

// NewRandomToken returns a URL-safe random value suitable for the state and nonce
// parameters.
func NewRandomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func userFromIDToken(token *oidc.IDToken) (*User, error) {
	var claims map[string]any
	if err := token.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse id token claims: %w", err)
	}

	username := usernameFromClaims(claims, token.Subject)
	if username == "" {
		return nil, ErrAuthenticationFailed
	}

	return &User{
		Username:   username,
		Attributes: attributesFromClaims(claims),
	}, nil
}

// usernameFromClaims picks the claim that identifies the user's GT account. Providers
// differ in which claim they release, and some put a full email address in
// preferred_username, so the local part is used when one is present.
func usernameFromClaims(claims map[string]any, subject string) string {
	for _, name := range []string{"username", "preferred_username", "email"} {
		candidate := stringClaim(claims, name)
		if candidate == "" {
			continue
		}
		if at := strings.Index(candidate, "@"); at > 0 {
			candidate = candidate[:at]
		}
		return candidate
	}

	return strings.TrimSpace(subject)
}

func stringClaim(claims map[string]any, name string) string {
	value, ok := claims[name].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

// attributesFromClaims flattens the scalar claims so the frontend can show them.
// Token bookkeeping claims that are credentials or hashes are left out.
func attributesFromClaims(claims map[string]any) map[string]string {
	attributes := make(map[string]string, len(claims))

	for name, value := range claims {
		switch name {
		case "nonce", "at_hash", "c_hash":
			continue
		}

		switch typed := value.(type) {
		case string:
			attributes[name] = typed
		case bool:
			attributes[name] = strconv.FormatBool(typed)
		case float64:
			attributes[name] = strconv.FormatFloat(typed, 'f', -1, 64)
		}
	}

	return attributes
}
