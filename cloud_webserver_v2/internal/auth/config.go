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
	// DefaultFrontendURL is where users are sent back to after a successful login.
	DefaultFrontendURL = "http://localhost:5173"
	// DefaultSessionCookieName is the name of the cookie holding our own opaque session id.
	DefaultSessionCookieName = "hytech_session"
	// DefaultSessionTTL keeps a login valid for half a day.
	DefaultSessionTTL = 12 * time.Hour
)

// Config holds the settings needed to integrate with Georgia Tech's CAS server.
type Config struct {
	// BaseURL is the CAS server, for example https://sso.gatech.edu/cas.
	BaseURL string
	// ServiceURL is the callback URL registered with Georgia Tech. It must be
	// identical for the login redirect and the ticket validation request.
	ServiceURL string
	// FrontendURL is where the browser is redirected after logging in or out.
	FrontendURL string
	// CookieName is the name of the session cookie we issue.
	CookieName string
	// CookieSecure marks the session cookie as HTTPS-only. It must be true everywhere
	// except local development over plain HTTP.
	CookieSecure bool
	// SessionTTL is how long a session stays valid.
	SessionTTL time.Duration
}

// ConfigFromEnv builds a Config from environment variables.
// GT_CAS_SERVICE_URL is required, everything else has sensible defaults.
// In the dev environment (ENV=dev) cookies are not marked secure so the server can be
// used over http://localhost, otherwise secure cookies are the default.
func ConfigFromEnv() (Config, error) {
	serviceURL := os.Getenv("GT_CAS_SERVICE_URL")
	if serviceURL == "" {
		return Config{}, errors.New("could not get GT_CAS_SERVICE_URL environment variable")
	}

	baseURL := os.Getenv("GT_CAS_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultCASBaseURL
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
			return Config{}, fmt.Errorf("could not parse SESSION_COOKIE_SECURE environment variable: %w", err)
		}
		cookieSecure = parsed
	}

	sessionTTL := DefaultSessionTTL
	if raw := os.Getenv("SESSION_TTL_HOURS"); raw != "" {
		hours, err := strconv.Atoi(raw)
		if err != nil || hours <= 0 {
			return Config{}, errors.New("SESSION_TTL_HOURS environment variable must be a positive integer")
		}
		sessionTTL = time.Duration(hours) * time.Hour
	}

	return Config{
		BaseURL:      baseURL,
		ServiceURL:   serviceURL,
		FrontendURL:  frontendURL,
		CookieName:   cookieName,
		CookieSecure: cookieSecure,
		SessionTTL:   sessionTTL,
	}, nil
}
