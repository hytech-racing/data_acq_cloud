package auth

import (
	"reflect"
	"strings"
	"testing"
)

func TestAuthConfigFromEnvRequiresOIDCSettings(t *testing.T) {
	tests := []struct {
		name    string
		unset   string
		wantErr string
	}{
		{name: "issuer", unset: "GT_OIDC_ISSUER_URL", wantErr: "GT_OIDC_ISSUER_URL"},
		{name: "client id", unset: "GT_OIDC_CLIENT_ID", wantErr: "GT_OIDC_CLIENT_ID"},
		{name: "redirect url", unset: "GT_OIDC_REDIRECT_URL", wantErr: "GT_OIDC_REDIRECT_URL"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GT_OIDC_ISSUER_URL", "https://issuer.example.edu")
			t.Setenv("GT_OIDC_CLIENT_ID", "client-id")
			t.Setenv("GT_OIDC_REDIRECT_URL", "https://app.example.edu/auth/callback")
			t.Setenv(test.unset, "")

			if _, err := AuthConfigFromEnv(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("AuthConfigFromEnv() error = %v, want it to mention %s", err, test.wantErr)
			}
		})
	}
}

func TestAuthConfigFromEnvReadsOIDCSettings(t *testing.T) {
	t.Setenv("GT_OIDC_ISSUER_URL", "https://issuer.example.edu")
	t.Setenv("GT_OIDC_CLIENT_ID", "client-id")
	t.Setenv("GT_OIDC_CLIENT_SECRET", "client-secret")
	t.Setenv("GT_OIDC_REDIRECT_URL", "https://app.example.edu/auth/callback")
	t.Setenv("GT_OIDC_SCOPES", "profile,email")
	t.Setenv("FRONTEND_URL", "")

	config, err := AuthConfigFromEnv()
	if err != nil {
		t.Fatalf("AuthConfigFromEnv() returned unexpected error: %v", err)
	}

	if config.OIDCIssuerURL != "https://issuer.example.edu" {
		t.Errorf("OIDCIssuerURL = %q", config.OIDCIssuerURL)
	}
	if config.OIDCClientID != "client-id" {
		t.Errorf("OIDCClientID = %q", config.OIDCClientID)
	}
	if config.OIDCClientSecret != "client-secret" {
		t.Errorf("OIDCClientSecret = %q", config.OIDCClientSecret)
	}
	if config.OIDCRedirectURL != "https://app.example.edu/auth/callback" {
		t.Errorf("OIDCRedirectURL = %q", config.OIDCRedirectURL)
	}
	if want := []string{"openid", "profile", "email"}; !reflect.DeepEqual(config.OIDCScopes, want) {
		t.Errorf("OIDCScopes = %v, want %v", config.OIDCScopes, want)
	}
	if config.FrontendURL != DefaultFrontendURL {
		t.Errorf("FrontendURL = %q, want %q", config.FrontendURL, DefaultFrontendURL)
	}
}

func TestParseScopesAlwaysIncludesOpenID(t *testing.T) {
	if got, want := parseScopes(""), strings.Fields(DefaultOIDCScopes); !reflect.DeepEqual(got, want) {
		t.Errorf("parseScopes(\"\") = %v, want %v", got, want)
	}

	if got, want := parseScopes("profile email profile"), []string{"openid", "profile", "email"}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseScopes() = %v, want %v", got, want)
	}
}
