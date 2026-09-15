package http

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/auth"
	"github.com/hytech-racing/cloud-webserver-v2/internal/logging"
)

// authHandler implements the Georgia Tech CAS single sign-on flow.
//
// The browser is sent to the CAS login page, CAS sends it back to our callback
// with a one-time service ticket, we validate that ticket against CAS server-side,
// and finally we issue our own session cookie. The ticket is never stored.
type authHandler struct {
	config   auth.Config
	cas      *auth.CASClient
	sessions auth.SessionStore
}

// NewAuthHandler registers the Georgia Tech SSO routes:
//
//	GET /auth/login    -> redirect to the CAS login page
//	GET /auth/callback -> validate the returned ticket and start a session
//	GET /auth/logout   -> destroy the local session
//	GET /auth/me       -> return the currently authenticated user
func NewAuthHandler(r *chi.Mux, config auth.Config, sessions auth.SessionStore) {
	handler := &authHandler{
		config:   config,
		cas:      auth.NewCASClient(config.BaseURL, config.ServiceURL),
		sessions: sessions,
	}

	r.Route("/auth", func(r chi.Router) {
		r.Get("/login", handler.Login)
		r.Get("/callback", HandlerFunc(handler.Callback).ServeHTTP)
		r.Get("/logout", handler.Logout)
		r.Get("/me", HandlerFunc(handler.Me).ServeHTTP)
	})
}

// Login redirects the browser to GT CAS. The service parameter is our callback URL,
// which must be registered with Georgia Tech.
func (handler *authHandler) Login(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, handler.cas.LoginURL(), http.StatusFound)
}

// Callback receives the one-time service ticket from CAS, validates it, and on success
// starts a session and returns the user to the frontend.
func (handler *authHandler) Callback(w http.ResponseWriter, r *http.Request) *HandlerError {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		logging.GetLogger().Warn("cas callback received without a service ticket")
		http.Redirect(w, r, handler.errorRedirect("missing_ticket"), http.StatusFound)
		return nil
	}

	// The ticket is validated immediately: it is single-use and expires within seconds.
	// It is never logged or persisted.
	user, err := handler.cas.Validate(r.Context(), ticket)
	if err != nil {
		logging.GetLogger().ErrorF("cas ticket validation failed: %v", redactTicket(err.Error(), ticket))
		http.Redirect(w, r, handler.errorRedirect("auth_failed"), http.StatusFound)
		return nil
	}

	session, err := handler.sessions.Create(user)
	if err != nil {
		return NewHandlerError("failed to create session: "+err.Error(), http.StatusInternalServerError)
	}

	http.SetCookie(w, handler.sessionCookie(session))
	logging.GetLogger().InfoF("user %s logged in through georgia tech sso", user.Username)
	http.Redirect(w, r, handler.config.FrontendURL, http.StatusFound)
	return nil
}

// Logout deletes the local session and clears the cookie. It does not end the user's
// CAS session; users stay signed in to other Georgia Tech apps.
func (handler *authHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(handler.config.CookieName); err == nil {
		handler.sessions.Delete(cookie.Value)
	}

	http.SetCookie(w, handler.expiredSessionCookie())
	http.Redirect(w, r, handler.config.FrontendURL, http.StatusFound)
}

// Me returns the user associated with the current session, or 401 when there is none.
func (handler *authHandler) Me(w http.ResponseWriter, r *http.Request) *HandlerError {
	session, err := handler.currentSession(r)
	if err != nil {
		return NewHandlerError("not authenticated", http.StatusUnauthorized)
	}

	response := make(map[string]interface{})
	response["message"] = nil
	response["data"] = session.User
	render.JSON(w, r, response)
	return nil
}

// RequireAuth can be used to protect routes that should only be reachable by a
// logged in Georgia Tech user.
func (handler *authHandler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := handler.currentSession(r); err != nil {
			handleHTTPError(w, *NewHandlerError("not authenticated", http.StatusUnauthorized))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// currentSession looks up the session referenced by the request's session cookie.
func (handler *authHandler) currentSession(r *http.Request) (*auth.Session, error) {
	cookie, err := r.Cookie(handler.config.CookieName)
	if err != nil {
		return nil, auth.ErrSessionNotFound
	}
	return handler.sessions.Get(cookie.Value)
}

func (handler *authHandler) sessionCookie(session *auth.Session) *http.Cookie {
	return &http.Cookie{
		Name:     handler.config.CookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   handler.config.CookieSecure,
		SameSite: handler.sameSite(),
		Expires:  session.ExpiresAt,
	}
}

func (handler *authHandler) expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     handler.config.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   handler.config.CookieSecure,
		SameSite: handler.sameSite(),
		MaxAge:   -1,
	}
}

// sameSite returns None for secure cookies so the cookie still works when the
// frontend is served from a different site (for example github.io), and Lax over
// plain HTTP where browsers reject SameSite=None cookies.
func (handler *authHandler) sameSite() http.SameSite {
	if handler.config.CookieSecure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

// errorRedirect sends the user back to the frontend with an error code it can display.
func (handler *authHandler) errorRedirect(code string) string {
	frontendURL, err := url.Parse(handler.config.FrontendURL)
	if err != nil {
		return handler.config.FrontendURL
	}

	query := frontendURL.Query()
	query.Set("error", code)
	frontendURL.RawQuery = query.Encode()
	return frontendURL.String()
}

// redactTicket removes a service ticket from a message before it is logged. CAS failure
// messages and transport errors can both echo the ticket, which is a credential.
func redactTicket(message, ticket string) string {
	if ticket == "" {
		return message
	}
	return strings.ReplaceAll(message, ticket, "[redacted]")
}
