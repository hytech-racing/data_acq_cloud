package auth

import (
	"testing"
	"time"
)

func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"GT_CAS_BASE_URL",
		"GT_CAS_SERVICE_URL",
		"FRONTEND_URL",
		"SESSION_COOKIE_NAME",
		"SESSION_COOKIE_SECURE",
		"SESSION_TTL_HOURS",
		"ENV",
	} {
		t.Setenv(name, "")
	}
}

func TestConfigFromEnvRequiresServiceURL(t *testing.T) {
	clearAuthEnv(t)

	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("ConfigFromEnv() = nil error, want an error when GT_CAS_SERVICE_URL is unset")
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("GT_CAS_SERVICE_URL", "https://yourapp.com/auth/callback")
	t.Setenv("ENV", "dev")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() returned unexpected error: %v", err)
	}

	if config.BaseURL != DefaultCASBaseURL {
		t.Errorf("BaseURL = %q, want %q", config.BaseURL, DefaultCASBaseURL)
	}
	if config.FrontendURL != DefaultFrontendURL {
		t.Errorf("FrontendURL = %q, want %q", config.FrontendURL, DefaultFrontendURL)
	}
	if config.CookieName != DefaultSessionCookieName {
		t.Errorf("CookieName = %q, want %q", config.CookieName, DefaultSessionCookieName)
	}
	if config.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, want %v", config.SessionTTL, DefaultSessionTTL)
	}
	if config.CookieSecure {
		t.Error("CookieSecure = true in dev, want false so the cookie works over http://localhost")
	}
}

func TestConfigFromEnvSecureCookiesOutsideDev(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("GT_CAS_SERVICE_URL", "https://yourapp.com/auth/callback")
	t.Setenv("ENV", "production")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() returned unexpected error: %v", err)
	}
	if !config.CookieSecure {
		t.Error("CookieSecure = false outside dev, want true")
	}
}

func TestConfigFromEnvOverrides(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("GT_CAS_SERVICE_URL", "https://yourapp.com/auth/callback")
	t.Setenv("GT_CAS_BASE_URL", "https://cas.example.com/cas")
	t.Setenv("FRONTEND_URL", "https://app.example.com")
	t.Setenv("SESSION_COOKIE_NAME", "custom_session")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SESSION_TTL_HOURS", "3")
	t.Setenv("ENV", "dev")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() returned unexpected error: %v", err)
	}

	if config.BaseURL != "https://cas.example.com/cas" {
		t.Errorf("BaseURL = %q", config.BaseURL)
	}
	if config.FrontendURL != "https://app.example.com" {
		t.Errorf("FrontendURL = %q", config.FrontendURL)
	}
	if config.CookieName != "custom_session" {
		t.Errorf("CookieName = %q", config.CookieName)
	}
	if !config.CookieSecure {
		t.Error("CookieSecure = false, want it overridden to true")
	}
	if config.SessionTTL != 3*time.Hour {
		t.Errorf("SessionTTL = %v, want %v", config.SessionTTL, 3*time.Hour)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	t.Run("session ttl", func(t *testing.T) {
		clearAuthEnv(t)
		t.Setenv("GT_CAS_SERVICE_URL", "https://yourapp.com/auth/callback")
		t.Setenv("SESSION_TTL_HOURS", "not-a-number")

		if _, err := ConfigFromEnv(); err == nil {
			t.Error("ConfigFromEnv() = nil error, want an error for an invalid SESSION_TTL_HOURS")
		}
	})

	t.Run("cookie secure", func(t *testing.T) {
		clearAuthEnv(t)
		t.Setenv("GT_CAS_SERVICE_URL", "https://yourapp.com/auth/callback")
		t.Setenv("SESSION_COOKIE_SECURE", "sometimes")

		if _, err := ConfigFromEnv(); err == nil {
			t.Error("ConfigFromEnv() = nil error, want an error for an invalid SESSION_COOKIE_SECURE")
		}
	})
}
