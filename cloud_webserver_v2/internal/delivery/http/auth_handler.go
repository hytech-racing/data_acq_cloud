package http

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/hytech-racing/cloud-webserver-v2/internal/auth"
)

const usernamesFile = "usernames.json"

// ticketReplayWindow is how long the outcome of a validated service ticket is replayed.
// CAS service tickets are single-use, so a browser that visits the callback more than once
// for a single login must not have to exchange the ticket a second time.
const ticketReplayWindow = time.Minute

// AuthHandler implements the Georgia Tech CAS single sign-on flow.
//
// The browser is sent to the CAS login page, CAS sends it back to our callback
// with a one-time service ticket, we validate that ticket against CAS server-side,
// and finally we issue our own session cookie. The ticket is never stored.
type AuthHandler struct {
	config   auth.Config
	cas      *auth.CASClient
	tickets  *auth.TicketExchange
	sessions auth.SessionStore
	fileLock sync.RWMutex
}

// usernameRequest is the JSON body accepted by RegisterEmail
type usernameRequest struct {
	Username string `json:"username"`
}

// NewAuthHandler registers the Georgia Tech SSO routes:
//
//	GET /auth/login    -> redirect to the CAS login page
//	GET /auth/callback -> validate the returned ticket and start a session
//	GET /auth/logout   -> destroy the local session
//	GET /auth/me       -> return the currently authenticated user
//
// Only emails present in the email allowlist may start a session; everyone else is
// sent back to the frontend with an email_not_allowed error.
func NewAuthHandler(r *chi.Mux, config auth.Config, sessions auth.SessionStore) *AuthHandler {
	cas := auth.NewCASClient(config.BaseURL, config.ServiceURL)

	handler := &AuthHandler{
		config:   config,
		cas:      cas,
		tickets:  auth.NewTicketExchange(ticketReplayWindow, cas.Validate),
		sessions: sessions,
		fileLock: sync.RWMutex{},
	}

	r.Route("/auth", func(r chi.Router) {
		r.Get("/login", handler.Login)
		r.Get("/callback", HandlerFunc(handler.Callback).ServeHTTP)
		r.Get("/logout", handler.Logout)
		r.Get("/me", HandlerFunc(handler.Me).ServeHTTP)
		r.Get("/usernames", handler.AllowedUsernames)
		r.Post("/usernames", HandlerFunc(handler.RegisterUsername).ServeHTTP)
		r.Delete("/usernames", HandlerFunc(handler.RemoveUsername).ServeHTTP)
	})

	return handler
}

// Login redirects the browser to GT CAS. The service parameter is our callback URL,
// which must be registered with Georgia Tech.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.cas.LoginURL(), http.StatusFound)
}

// Callback receives the one-time service ticket from CAS, validates it, and on success
// starts a session and returns the user to the frontend. A session is only started when
// the email CAS released is present in the allowlist.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) *HandlerError {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		log.Printf("cas callback received without a service ticket")
		http.Redirect(w, r, h.errorRedirect("missing_ticket"), http.StatusFound)
		return nil
	}

	// The ticket is exchanged immediately: it is single-use and expires within seconds.
	// It is never logged or persisted. Repeats of the same ticket, which happen when a
	// browser requests the callback more than once, replay the first outcome.
	user, err := h.tickets.Exchange(r.Context(), ticket)
	if err != nil {
		log.Printf("cas ticket validation failed: %v", redactTicket(err.Error(), ticket))
		http.Redirect(w, r, h.errorRedirect("auth_failed"), http.StatusFound)
		return nil
	}

	log.Printf("Hello there!!!!!!!!")

	allowed := h.ContainsUsername(user.Username)
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
	log.Printf("user %s logged in through georgia tech sso", user.Username)
	http.Redirect(w, r, h.config.FrontendURL, http.StatusFound)
	return nil
}

// Logout deletes the local session and clears the cookie. It does not end the user's
// CAS session; users stay signed in to other Georgia Tech apps.
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

	h.fileLock.Lock()
	defer h.fileLock.Unlock()

	var allowedUsernames []string
	raw, err := os.ReadFile(usernamesFile)
	if err != nil {
		allowedUsernames = []string{request.Username}
		os.WriteFile(usernamesFile, []byte("[\""+request.Username+"\"]"), 0644)
		w.WriteHeader(http.StatusCreated)
		return nil
	}

	err = json.Unmarshal(raw, &allowedUsernames)
	if err != nil {
		panic(err)
	}
	allowedUsernames = append(allowedUsernames, request.Username)
	data, err := json.MarshalIndent(allowedUsernames, "", "  ")
	if err != nil {
		panic(err)
	}
	os.WriteFile(usernamesFile, data, 0644)
	w.WriteHeader(http.StatusCreated)

	return nil
}

func (h *AuthHandler) RemoveUsername(w http.ResponseWriter, r *http.Request) *HandlerError {
	_, err := h.CurrentSession(r)
	if err != nil {
		return NewHandlerError("not authenticated", http.StatusUnauthorized)
	}

	var request usernameRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return NewHandlerError("invalid request body, expected a JSON object with a username field", http.StatusBadRequest)
	}

	h.fileLock.Lock()
	defer h.fileLock.Unlock()

	var allowedUsernames []string
	raw, err := os.ReadFile(usernamesFile)
	if err != nil {
		return NewHandlerError("Usernames file could not be read.", http.StatusBadRequest)
	}
	
	err = json.Unmarshal(raw, &allowedUsernames)
	if err != nil {
		return NewHandlerError("Usernames file could not be read.", http.StatusBadRequest)
	}
	modifiedUsernames := allowedUsernames[:0]
	for _, username := range allowedUsernames {
		if (username != request.Username) {
			modifiedUsernames = append(modifiedUsernames, username)
		}
	}

	data, err := json.MarshalIndent(modifiedUsernames, "", "  ")
	if err != nil {
		panic(err)
	}
	os.WriteFile(usernamesFile, data, 0644)
	w.WriteHeader(http.StatusCreated)

	return nil
}

func (h *AuthHandler) AllowedUsernames(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, h.getAllowedUsernames())
}

func (h *AuthHandler) ContainsUsername(username string) bool {
	return slices.Contains(h.getAllowedUsernames(), username)
}

func (h *AuthHandler) getAllowedUsernames() []string {
	h.fileLock.RLock()
	defer h.fileLock.RUnlock()

	data, err := os.ReadFile(usernamesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("getAllowedEmails: failed to read file: %v", err)
		}
		return []string{}
	}

	var usernames []string
	if err := json.Unmarshal(data, &usernames); err != nil {
		log.Printf("getAllowedEmails: failed to unmarshal usernames: %v", err)
		return []string{}
	}

	log.Printf("usernames: %v", usernames)

	return usernames
}

// CurrentSession looks up the session referenced by the request's session cookie.
func (h *AuthHandler) CurrentSession(r *http.Request) (*auth.Session, error) {
	cookie, err := r.Cookie(h.config.CookieName)
	if err != nil {
		return nil, auth.ErrSessionNotFound
	}
	return h.sessions.Get(cookie.Value)
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

// redactTicket removes a service ticket from a message before it is logged. CAS failure
// messages and transport errors can both echo the ticket, which is a credential.
func redactTicket(message, ticket string) string {
	if ticket == "" {
		return message
	}
	return strings.ReplaceAll(message, ticket, "[redacted]")
}
