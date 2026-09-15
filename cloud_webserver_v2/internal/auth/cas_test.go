package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const testServiceURL = "https://yourapp.com/auth/callback"

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

func TestLoginURLEscapesServiceURL(t *testing.T) {
	client := NewCASClient("https://sso.gatech.edu/cas/", testServiceURL)

	want := "https://sso.gatech.edu/cas/login?service=" + url.QueryEscape(testServiceURL)
	if got := client.LoginURL(); got != want {
		t.Errorf("LoginURL() = %q, want %q", got, want)
	}
}

func TestLogoutURL(t *testing.T) {
	client := NewCASClient("https://sso.gatech.edu/cas", testServiceURL)

	if got, want := client.LogoutURL(""), "https://sso.gatech.edu/cas/logout"; got != want {
		t.Errorf("LogoutURL(\"\") = %q, want %q", got, want)
	}

	want := "https://sso.gatech.edu/cas/logout?service=" + url.QueryEscape(testServiceURL)
	if got := client.LogoutURL(testServiceURL); got != want {
		t.Errorf("LogoutURL(service) = %q, want %q", got, want)
	}
}

func TestValidateSendsTicketAndServiceAndParsesAttributes(t *testing.T) {
	var receivedTicket, receivedService string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != casValidatePath {
			t.Errorf("validation path = %q, want %q", r.URL.Path, casValidatePath)
		}
		receivedTicket = r.URL.Query().Get("ticket")
		receivedService = r.URL.Query().Get("service")
		w.Write([]byte(successResponse))
	}))
	defer server.Close()

	client := NewCASClient(server.URL, testServiceURL)

	user, err := client.Validate(context.Background(), "ST-12345-abcdefg-sso")
	if err != nil {
		t.Fatalf("Validate() returned unexpected error: %v", err)
	}

	if receivedTicket != "ST-12345-abcdefg-sso" {
		t.Errorf("ticket sent to CAS = %q, want %q", receivedTicket, "ST-12345-abcdefg-sso")
	}
	if receivedService != testServiceURL {
		t.Errorf("service sent to CAS = %q, want %q", receivedService, testServiceURL)
	}
	if user.Username != "gburdell3" {
		t.Errorf("Username = %q, want %q", user.Username, "gburdell3")
	}
	if got := user.Attribute("mail"); got != "george.burdell@gatech.edu" {
		t.Errorf("mail attribute = %q, want %q", got, "george.burdell@gatech.edu")
	}
	if got := user.Attribute("displayName"); got != "George P. Burdell" {
		t.Errorf("displayName attribute = %q, want %q", got, "George P. Burdell")
	}
}

func TestValidateEmptyTicketDoesNotCallCAS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("CAS should not be called when the ticket is empty")
	}))
	defer server.Close()

	client := NewCASClient(server.URL, testServiceURL)

	_, err := client.Validate(context.Background(), "  ")
	if err == nil {
		t.Fatal("Validate() = nil error, want an error for an empty ticket")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != "INVALID_TICKET" {
		t.Errorf("Validate() error = %v, want a ValidationError with code INVALID_TICKET", err)
	}
}

func TestValidateReturnsValidationErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(failureResponse))
	}))
	defer server.Close()

	client := NewCASClient(server.URL, testServiceURL)

	_, err := client.Validate(context.Background(), "ST-12345-abcdefg-sso")
	if err == nil {
		t.Fatal("Validate() = nil error, want an error")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Validate() error = %v, want a ValidationError", err)
	}
	if validationErr.Code != "INVALID_TICKET" {
		t.Errorf("ValidationError.Code = %q, want %q", validationErr.Code, "INVALID_TICKET")
	}
	if validationErr.Message != "Ticket ST-12345-abcdefg-sso not recognized" {
		t.Errorf("ValidationError.Message = %q", validationErr.Message)
	}
}

func TestValidateRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not xml at all"))
	}))
	defer server.Close()

	client := NewCASClient(server.URL, testServiceURL)

	if _, err := client.Validate(context.Background(), "ST-12345"); err == nil {
		t.Fatal("Validate() = nil error, want a parse error")
	}
}

func TestParseCASResponseSupportsNameValueAttributes(t *testing.T) {
	response := []byte(`<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas">
  <cas:authenticationSuccess>
    <cas:user>gburdell3</cas:user>
    <cas:attributes>
      <cas:attribute name="mail" value="george.burdell@gatech.edu"/>
      <cas:attribute name="displayName" value="George P. Burdell"/>
    </cas:attributes>
  </cas:authenticationSuccess>
</cas:serviceResponse>`)

	user, err := parseCASResponse(response)
	if err != nil {
		t.Fatalf("parseCASResponse() returned unexpected error: %v", err)
	}
	if got := user.Attribute("mail"); got != "george.burdell@gatech.edu" {
		t.Errorf("mail attribute = %q, want %q", got, "george.burdell@gatech.edu")
	}
	if got := user.Attribute("displayName"); got != "George P. Burdell" {
		t.Errorf("displayName attribute = %q, want %q", got, "George P. Burdell")
	}
}

func TestParseCASResponseWithoutUserFails(t *testing.T) {
	_, err := parseCASResponse([]byte(`<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas"></cas:serviceResponse>`))
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Errorf("parseCASResponse() error = %v, want ErrAuthenticationFailed", err)
	}
}
