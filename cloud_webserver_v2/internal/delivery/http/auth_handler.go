package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/auth"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database/usecase"
)

const (
	// loginCookiePrefix is followed by the login's state, which keeps concurrent logins
	// in different browser tabs from overwriting each other.
	loginCookiePrefix = "hytech_oidc_login_"
	// loginCookieTTL bounds how long an unfinished login can wait at the provider.
	loginCookieTTL = 10 * time.Minute
)

// AuthHandler implements the Georgia Tech single sign-on flow using OpenID Connect.
//
// The browser is sent to the provider's authorization endpoint with a state, nonce and
// PKCE challenge. The provider sends it back to our callback with a one-time
// authorization code, we exchange that code server-side for a verified id token, and
// finally we issue our own session cookie. The code is never stored.
type AuthHandler struct {
	config   auth.AuthConfig
	oidc     *auth.OIDCClient
	sessions auth.SessionStore
	dbClient *database.DatabaseClient
	router   *chi.Mux
}

// usernameRequest is the JSON body accepted by RegisterEmail
type usernameRequest struct {
	Username string `json:"username"`
}

// NewAuthHandler registers the Georgia Tech SSO routes:
//
//	GET /auth/login    -> redirect to the OIDC authorization endpoint
//	GET /auth/callback -> exchange the returned code and start a session
//	GET /auth/logout   -> destroy the local session
//	GET /auth/me       -> return the currently authenticated user
//
// Only usernames present in the allowlist may start a session; everyone else is sent
// back to the frontend with a user_not_allowed error.
func NewAuthHandler(ctx context.Context, r *chi.Mux, config auth.AuthConfig, sessions auth.SessionStore, dbClient *database.DatabaseClient) (*AuthHandler, error) {
	oidcClient, err := auth.NewOIDCClient(
		ctx,
		config.OIDCIssuerURL,
		config.OIDCClientID,
		config.OIDCClientSecret,
		config.OIDCRedirectURL,
		config.OIDCScopes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize oidc client: %w", err)
	}

	handler := &AuthHandler{
		config:   config,
		oidc:     oidcClient,
		sessions: sessions,
		dbClient: dbClient,
		router:   r,
	}

	return handler, nil
}

func (h *AuthHandler) RegisterNonCallbackEndpoints() {
	h.router.Route("/auth", func(r chi.Router) {
		r.Get("/login", h.Login)
		r.Get("/logout", h.Logout)
		r.Get("/me", HandlerFunc(h.Me).ServeHTTP)
		r.Get("/usernames", h.AllowedUsernames)
		r.Post("/usernames", HandlerFunc(h.RegisterUsername).ServeHTTP)
		r.Delete("/usernames", HandlerFunc(h.RemoveUsername).ServeHTTP)
	})
}

// Login redirects the browser to the provider. The redirect URL is the callback
// registered with Georgia Tech as GT_OIDC_REDIRECT_URL.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state, err := auth.NewRandomToken()
	if err != nil {
		log.Printf("failed to generate oidc state: %v", err)
		http.Error(w, "failed to start login", http.StatusInternalServerError)
		return
	}

	nonce, err := auth.NewRandomToken()
	if err != nil {
		log.Printf("failed to generate oidc nonce: %v", err)
		http.Error(w, "failed to start login", http.StatusInternalServerError)
		return
	}

	verifier := auth.NewCodeVerifier()
	http.SetCookie(w, h.loginCookie(state, nonce, verifier))
	http.Redirect(w, r, h.oidc.AuthCodeURL(state, nonce, verifier), http.StatusFound)
}

// Callback receives the one-time authorization code, exchanges it for a verified id
// token, and on success starts a session and returns the user to the frontend. A session
// is only started when the username released by the provider is present in the allowlist.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) *HandlerError {
	query := r.URL.Query()

	if providerError := query.Get("error"); providerError != "" {
		log.Printf("oidc provider returned an error: %s: %s", providerError, query.Get("error_description"))
		http.Redirect(w, r, h.errorRedirect("auth_failed"), http.StatusFound)
		return nil
	}

	code := query.Get("code")
	state := query.Get("state")
	if code == "" || state == "" || len(state) > 256 {
		log.Printf("oidc callback received without an authorization code or state")
		http.Redirect(w, r, h.errorRedirect("missing_code"), http.StatusFound)
		return nil
	}

	nonce, verifier, ok := h.loginCredentials(r, state)
	if !ok {
		// A reload after a successful login no longer has the one-time login cookie but
		// does have a session cookie, so send the user on instead of showing an error.
		if _, err := h.CurrentSession(r); err == nil {
			http.Redirect(w, r, h.config.FrontendURL, http.StatusFound)
			return nil
		}

		log.Printf("oidc callback had an invalid or expired state")
		http.Redirect(w, r, h.errorRedirect("invalid_state"), http.StatusFound)
		return nil
	}

	// The login cookies are single-use and are cleared before the code is redeemed.
	http.SetCookie(w, h.expiredLoginCookie(state))

	// The code is exchanged immediately: it is single-use and expires within seconds.
	// It is never logged or persisted.
	user, err := h.oidc.Exchange(r.Context(), code, verifier, nonce)
	if err != nil {
		log.Printf("oidc code exchange failed: %v", redactCode(err.Error(), code))
		http.Redirect(w, r, h.errorRedirect("auth_failed"), http.StatusFound)
		return nil
	}

	allowed, err := h.ContainsUsername(r.Context(), user.Username)
	if err != nil {
		log.Printf("failed to check the username allowlist: %v", err)
		http.Redirect(w, r, h.errorRedirect("auth_failed"), http.StatusFound)
		return nil
	}
	if !allowed {
		log.Printf("rejected login for an username that is not on the allowlist")
		http.Redirect(w, r, h.errorRedirect("user_not_allowed"), http.StatusFound)
		return nil
	}

	session, err := h.sessions.Create(user)
	if err != nil {
		return NewHandlerError("failed to create session: "+err.Error(), http.StatusInternalServerError)
	}

	http.SetCookie(w, h.sessionCookie(session))
	log.Printf("user %s logged in through georgia tech sso via oidc", user.Username)
	http.Redirect(w, r, h.config.FrontendURL, http.StatusFound)
	return nil
}

// Logout deletes the local session and clears the cookie. It does not end the user's
// session at the identity provider; users stay signed in to other Georgia Tech apps.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(h.config.CookieName); err == nil {
		h.sessions.Delete(cookie.Value)
	}

	http.SetCookie(w, h.expiredSessionCookie())
	http.Redirect(w, r, h.config.FrontendURL, http.StatusFound)
}

// Me returns the user associated with the current session, or 401 when there is none.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) *HandlerError {
	session, err := h.CurrentSession(r)
	if err != nil {
		return NewHandlerError("not authenticated", http.StatusUnauthorized)
	}

	response := make(map[string]interface{})
	response["message"] = nil
	response["data"] = session.User
	render.JSON(w, r, response)
	return nil
}

// RegisterUsername adds an username to the allowlist so that it is allowed to log in
func (h *AuthHandler) RegisterUsername(w http.ResponseWriter, r *http.Request) *HandlerError {
	_, err := h.CurrentSession(r)
	if err != nil {
		return NewHandlerError("not authenticated", http.StatusUnauthorized)
	}

	var request usernameRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return NewHandlerError("invalid request body, expected a JSON object with a username field", http.StatusBadRequest)
	}

	if strings.TrimSpace(request.Username) == "" {
		return NewHandlerError("username is required", http.StatusBadRequest)
	}
	for _, r := range request.Username {
		if unicode.IsUpper(r) {
			return NewHandlerError("usernames can only contain lowercase letters and numbers.", http.StatusBadRequest)
		}
	}

	err = h.dbClient.AllowedUsernameUseCase().AddUsername(r.Context(), request.Username)
	if errors.Is(err, usecase.ErrUsernameAlreadyExists) {
		return NewHandlerError("user is already an admin", http.StatusBadRequest)
	}
	if err != nil {
		log.Printf("failed to add username to the allowlist: %v", err)
		return NewHandlerError("failed to add username to the allowlist", http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusCreated)
	return nil
}

func (h *AuthHandler) RemoveUsername(w http.ResponseWriter, r *http.Request) *HandlerError {
	session, err := h.CurrentSession(r)
	if err != nil {
		return NewHandlerError("not authenticated", http.StatusUnauthorized)
	}

	var request usernameRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return NewHandlerError("invalid request body, expected a JSON object with a username field", http.StatusBadRequest)
	}

	if session.User.Username == request.Username {
		return NewHandlerError("Cannot delete self from admin list", http.StatusBadRequest)
	}

	err = h.dbClient.AllowedUsernameUseCase().RemoveUsername(r.Context(), request.Username)
	if err != nil {
		log.Printf("failed to remove username from the allowlist: %v", err)
		return NewHandlerError("failed to remove username from the allowlist", http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusCreated)
	return nil
}

func (h *AuthHandler) AllowedUsernames(w http.ResponseWriter, r *http.Request) {
	usernames, err := h.dbClient.AllowedUsernameUseCase().GetAllUsernames(r.Context())
	if err != nil {
		log.Printf("failed to read the username allowlist: %v", err)
		usernames = []string{}
	}

	render.JSON(w, r, usernames)
}

func (h *AuthHandler) ContainsUsername(ctx context.Context, username string) (bool, error) {
	return h.dbClient.AllowedUsernameUseCase().ContainsUsername(ctx, username)
}

// CurrentSession looks up the session referenced by the request's session cookie.
func (h *AuthHandler) CurrentSession(r *http.Request) (*auth.Session, error) {
	cookie, err := r.Cookie(h.config.CookieName)
	if err != nil {
		return nil, auth.ErrSessionNotFound
	}
	return h.sessions.Get(cookie.Value)
}

// loginCookie stores the nonce and PKCE verifier for one in-flight login. The state is
// part of the cookie name so several logins can be in progress at once, for example in
// different browser tabs. SameSite=Lax is enough because the provider returns with a
// top-level GET redirect.
func (h *AuthHandler) loginCookie(state, nonce, verifier string) *http.Cookie {
	return &http.Cookie{
		Name:     loginCookiePrefix + state,
		Value:    nonce + "." + verifier,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(loginCookieTTL.Seconds()),
	}
}

func (h *AuthHandler) expiredLoginCookie(state string) *http.Cookie {
	return &http.Cookie{
		Name:     loginCookiePrefix + state,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

// loginCredentials returns the nonce and PKCE verifier saved by Login. Looking the
// cookie up by state makes the state check implicit: a callback with an unknown state
// simply has no matching cookie.
func (h *AuthHandler) loginCredentials(r *http.Request, state string) (nonce, verifier string, ok bool) {
	cookie, err := r.Cookie(loginCookiePrefix + state)
	if err != nil {
		return "", "", false
	}

	nonce, verifier, found := strings.Cut(cookie.Value, ".")
	if !found || nonce == "" || verifier == "" {
		return "", "", false
	}

	return nonce, verifier, true
}

func (h *AuthHandler) sessionCookie(session *auth.Session) *http.Cookie {
	return &http.Cookie{
		Name:     h.config.CookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: h.sameSite(),
		Expires:  session.ExpiresAt,
	}
}

func (h *AuthHandler) expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     h.config.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: h.sameSite(),
		MaxAge:   -1,
	}
}

// sameSite returns None for secure cookies so the cookie still works when the
// frontend is served from a different site (for example github.io), and Lax over
// plain HTTP where browsers reject SameSite=None cookies.
func (h *AuthHandler) sameSite() http.SameSite {
	if h.config.CookieSecure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

// errorRedirect sends the user back to the frontend with an error code it can display.
func (h *AuthHandler) errorRedirect(code string) string {
	frontendURL, err := url.Parse(h.config.FrontendURL)
	if err != nil {
		return h.config.FrontendURL
	}

	query := frontendURL.Query()
	query.Set("error", code)
	frontendURL.RawQuery = query.Encode()
	return frontendURL.String()
}

// redactCode removes an authorization code from a message before it is logged. Provider
// failure messages and transport errors can both echo the code, which is a credential.
func redactCode(message, code string) string {
	if code == "" {
		return message
	}
	return strings.ReplaceAll(message, code, "[redacted]")
}
