package auth

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultFrontendURL = "http://localhost:5173"
	DefaultSessionCookieName = "hytech_session"
	DefaultSessionTTL = 12 * time.Hour
	DefaultOIDCScopes = "openid profile email"
)

type AuthConfig struct {
	// OIDCIssuerURL is the issuer URL of the identity provider. Discovery metadata is
	// read from <issuer>/.well-known/openid-configuration at startup.
	OIDCIssuerURL string
	// OIDCClientID is the client identifier registered with Georgia Tech.
	OIDCClientID string
	// OIDCClientSecret is the client secret registered with Georgia Tech. It may be
	// empty for a public client that only relies on PKCE.
	OIDCClientSecret string
	// OIDCRedirectURL is the callback URL registered with Georgia Tech. It must be
	// identical to the URL sent in the authorization request.
	OIDCRedirectURL string
	// OIDCScopes are the scopes requested during login. "openid" is always included.
	OIDCScopes []string
	FrontendURL string
	CookieName string
	// CookieSecure marks the session cookie as HTTPS-only. It must be true everywhere
	// except local development over plain HTTP.
	CookieSecure bool
	SessionTTL time.Duration
}

func AuthConfigFromEnv() (AuthConfig, error) {
	issuerURL := strings.TrimSpace(os.Getenv("GT_OIDC_ISSUER_URL"))
	if issuerURL == "" {
		return AuthConfig{}, errors.New("could not get GT_OIDC_ISSUER_URL environment variable")
	}

	clientID := strings.TrimSpace(os.Getenv("GT_OIDC_CLIENT_ID"))
	if clientID == "" {
		return AuthConfig{}, errors.New("could not get GT_OIDC_CLIENT_ID environment variable")
	}

	redirectURL := strings.TrimSpace(os.Getenv("GT_OIDC_REDIRECT_URL"))
	if redirectURL == "" {
		return AuthConfig{}, errors.New("could not get GT_OIDC_REDIRECT_URL environment variable")
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = DefaultFrontendURL
	}

	cookieName := os.Getenv("SESSION_COOKIE_NAME")
	if cookieName == "" {
		cookieName = DefaultSessionCookieName
	}

	cookieSecure := !strings.EqualFold(os.Getenv("ENV"), "dev")
	if raw := os.Getenv("SESSION_COOKIE_SECURE"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return AuthConfig{}, fmt.Errorf("could not parse SESSION_COOKIE_SECURE environment variable: %w", err)
		}
		cookieSecure = parsed
	}

	sessionTTL := DefaultSessionTTL
	if raw := os.Getenv("SESSION_TTL_HOURS"); raw != "" {
		hours, err := strconv.Atoi(raw)
		if err != nil || hours <= 0 {
			return AuthConfig{}, errors.New("SESSION_TTL_HOURS environment variable must be a positive integer")
		}
		sessionTTL = time.Duration(hours) * time.Hour
	}

	return AuthConfig{
		OIDCIssuerURL:    issuerURL,
		OIDCClientID:     clientID,
		OIDCClientSecret: strings.TrimSpace(os.Getenv("GT_OIDC_CLIENT_SECRET")),
		OIDCRedirectURL:  redirectURL,
		OIDCScopes:       parseScopes(os.Getenv("GT_OIDC_SCOPES")),
		FrontendURL:      frontendURL,
		CookieName:       cookieName,
		CookieSecure:     cookieSecure,
		SessionTTL:       sessionTTL,
	}, nil
}

// parseScopes accepts space or comma separated scopes and guarantees that "openid" is
// requested, since without it the provider does not issue an id token.
func parseScopes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		raw = DefaultOIDCScopes
	}

	seen := make(map[string]bool)
	scopes := make([]string, 0, 4)
	for _, scope := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n'
	}) {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}

	if !seen["openid"] {
		scopes = append([]string{"openid"}, scopes...)
	}

	return scopes
}
