package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/auth"
)

func TestLogoutClearsSessionAndRedirects(t *testing.T) {
	const frontendURL = "https://frontend.example/query-frontend/"
	const cookieName = "hytech_session"
	store := auth.NewMemorySessionStore(time.Hour)
	session, err := store.Create(&auth.User{Username: "test-user"})
	if err != nil {
		t.Fatal(err)
	}
	h := &AuthHandler{
		config:   auth.AuthConfig{FrontendURL: frontendURL, CookieName: cookieName},
		sessions: store,
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: session.ID})
	recorder := httptest.NewRecorder()
	h.Logout(recorder, req)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound || response.Header.Get("Location") != frontendURL {
		t.Fatalf("got status %d and Location %q", response.StatusCode, response.Header.Get("Location"))
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("expected an expired session cookie, got %v", cookies)
	}
	if _, err := store.Get(session.ID); err != auth.ErrSessionNotFound {
		t.Fatalf("expected the session to be deleted, got %v", err)
	}
}
