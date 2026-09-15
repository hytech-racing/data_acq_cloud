package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hytech-racing/cloud-webserver-v2/internal/auth"
	"github.com/hytech-racing/cloud-webserver-v2/internal/logging"
)

const successResponse = `<?xml version="1.0" encoding="UTF-8"?>
<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas">
  <cas:authenticationSuccess>
    <cas:user>gburdell3</cas:user>
    <cas:attributes>
      <cas:mail>george.burdell@gatech.edu</cas:mail>
      <cas:displayName>George P. Burdell</cas:displayName>
    </cas:attributes>
  </cas:authenticationSuccess>
</cas:serviceResponse>`

const failureResponse = `<?xml version="1.0" encoding="UTF-8"?>
<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas">
  <cas:authenticationFailure code="INVALID_TICKET">
    Ticket ST-12345-abcdefg-sso not recognized
  </cas:authenticationFailure>
</cas:serviceResponse>`

func TestMain(m *testing.M) {
	logging.InitLogger(10)
	os.Exit(m.Run())
}

// newTestAuthHandler wires an auth handler against a fake CAS server so the full
// login flow can be exercised without talking to Georgia Tech.
func newTestAuthHandler(t *testing.T, casHandler http.HandlerFunc) (*chi.Mux, auth.Config, *auth.MemorySessionStore, *httptest.Server) {
	t.Helper()

	casServer := httptest.NewServer(casHandler)
	t.Cleanup(casServer.Close)

	config := auth.Config{
		BaseURL:      casServer.URL,
		ServiceURL:   "https://yourapp.com/auth/callback",
		FrontendURL:  "https://frontend.example.com/app",
		CookieName:   "test_session",
		CookieSecure: false,
		SessionTTL:   time.Hour,
	}

	sessions := auth.NewMemorySessionStore(config.SessionTTL)
	t.Cleanup(sessions.Close)

	router := chi.NewRouter()
	NewAuthHandler(router, config, sessions)

	return router, config, sessions, casServer
}

func TestAuthLoginRedirectsToCAS(t *testing.T) {
	router, config, _, casServer := newTestAuthHandler(t, func(w http.ResponseWriter, r *http.Request) {})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/login", nil))

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}

	want := casServer.URL + "/login?service=" + url.QueryEscape(config.ServiceURL)
	if got := recorder.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestAuthCallbackCreatesSessionAndCookie(t *testing.T) {
	router, config, sessions, _ := newTestAuthHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ticket"); got != "ST-12345" {
			t.Errorf("ticket sent to CAS = %q, want %q", got, "ST-12345")
		}
		w.Write([]byte(successResponse))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/callback?ticket=ST-12345", nil))

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if got := recorder.Header().Get("Location"); got != config.FrontendURL {
		t.Errorf("Location = %q, want %q", got, config.FrontendURL)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != config.CookieName || cookie.Value == "" {
		t.Errorf("session cookie = %q:%q, want a value for %q", cookie.Name, cookie.Value, config.CookieName)
	}
	if !cookie.HttpOnly {
		t.Error("session cookie must be HttpOnly so scripts cannot read it")
	}

	session, err := sessions.Get(cookie.Value)
	if err != nil {
		t.Fatalf("session was not stored: %v", err)
	}
	if session.User.Username != "gburdell3" {
		t.Errorf("session user = %q, want %q", session.User.Username, "gburdell3")
	}
}

func TestAuthCallbackWithInvalidTicketRedirectsWithError(t *testing.T) {
	router, config, _, _ := newTestAuthHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(failureResponse))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/callback?ticket=ST-expired", nil))

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if got := recorder.Header().Get("Location"); got != config.FrontendURL+"?error=auth_failed" {
		t.Errorf("Location = %q, want the frontend with an auth_failed error", got)
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Error("no session cookie should be issued when validation fails")
	}
}

func TestAuthCallbackWithoutTicketRedirectsWithError(t *testing.T) {
	router, config, _, _ := newTestAuthHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("CAS should not be called when no ticket is present")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/callback", nil))

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if got := recorder.Header().Get("Location"); got != config.FrontendURL+"?error=missing_ticket" {
		t.Errorf("Location = %q, want the frontend with a missing_ticket error", got)
	}
}

func TestAuthMe(t *testing.T) {
	router, _, _, _ := newTestAuthHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse))
	})

	// Anonymous requests are rejected.
	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want %d", anonymous.Code, http.StatusUnauthorized)
	}

	// Log in to obtain a session cookie.
	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/auth/callback?ticket=ST-12345", nil))
	cookie := callback.Result().Cookies()[0]

	authenticatedRequest := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	authenticatedRequest.AddCookie(cookie)
	authenticated := httptest.NewRecorder()
	router.ServeHTTP(authenticated, authenticatedRequest)

	if authenticated.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d", authenticated.Code, http.StatusOK)
	}
	body := authenticated.Body.String()
	if !strings.Contains(body, "gburdell3") {
		t.Errorf("body = %q, want it to contain the username", body)
	}
	if !strings.Contains(body, "george.burdell@gatech.edu") {
		t.Errorf("body = %q, want it to contain the released mail attribute", body)
	}
}

func TestAuthLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	router, config, sessions, _ := newTestAuthHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse))
	})

	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/auth/callback?ticket=ST-12345", nil))
	cookie := callback.Result().Cookies()[0]

	logoutRequest := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	logoutRequest.AddCookie(cookie)
	logout := httptest.NewRecorder()
	router.ServeHTTP(logout, logoutRequest)

	if logout.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", logout.Code, http.StatusFound)
	}
	if got := logout.Header().Get("Location"); got != config.FrontendURL {
		t.Errorf("Location = %q, want %q", got, config.FrontendURL)
	}

	clearedCookies := logout.Result().Cookies()
	if len(clearedCookies) != 1 || clearedCookies[0].MaxAge >= 0 {
		t.Errorf("logout should clear the session cookie, got %+v", clearedCookies)
	}

	if _, err := sessions.Get(cookie.Value); err == nil {
		t.Error("session should be deleted after logout")
	}
}

func TestRedactTicketRemovesTicketFromLoggedMessage(t *testing.T) {
	message := "cas validation failed: INVALID_TICKET: Ticket ST-12345-abcdefg-sso not recognized"

	redacted := redactTicket(message, "ST-12345-abcdefg-sso")
	if strings.Contains(redacted, "ST-12345-abcdefg-sso") {
		t.Errorf("redactTicket() = %q, want the service ticket removed", redacted)
	}
	if !strings.Contains(redacted, "[redacted]") {
		t.Errorf("redactTicket() = %q, want a redaction marker", redacted)
	}

	if got := redactTicket(message, ""); got != message {
		t.Error("redactTicket() with an empty ticket should leave the message unchanged")
	}
}
