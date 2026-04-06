package authentication

type Discovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

var DiscoveryConfig = Discovery{
	AuthorizationEndpoint: "https://sso.gatech.edu/cas/oidc/oidcAuthorize",
	TokenEndpoint:         "https://sso.gatech.edu/cas/oidc/oidcAccessToken",
	UserinfoEndpoint:      "https://sso.gatech.edu/cas/oidc/oidcProfile",
	JwksURI:               "https://sso.gatech.edu/cas/oidc/jwks",
	EndSessionEndpoint:    "https://sso.gatech.edu/cas/oidc/oidcLogout",
}
