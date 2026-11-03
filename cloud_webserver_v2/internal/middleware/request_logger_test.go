package hytech_middleware

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactTicketHidesTicketFromLoggedURI(t *testing.T) {
	request := httptest.NewRequest("GET", "/auth/callback?ticket=ST-12345-abcdefg-sso&foo=bar", nil)

	redacted := redactTicket(request)

	if strings.Contains(redacted.RequestURI, "ST-12345-abcdefg-sso") {
		t.Errorf("RequestURI = %q, want the service ticket redacted", redacted.RequestURI)
	}
	if !strings.Contains(strings.ToLower(redacted.RequestURI), "redacted") {
		t.Errorf("RequestURI = %q, want the redacted ticket to be marked", redacted.RequestURI)
	}
	if !strings.Contains(redacted.RequestURI, "foo=bar") {
		t.Errorf("RequestURI = %q, want other query parameters preserved", redacted.RequestURI)
	}

	// The original request must keep the real ticket so the callback can validate it.
	if got := request.URL.Query().Get(TicketQueryParam); got != "ST-12345-abcdefg-sso" {
		t.Errorf("original ticket = %q, want it unchanged", got)
	}
}

func TestRedactTicketLeavesOtherRequestsAlone(t *testing.T) {
	request := httptest.NewRequest("GET", "/mcaps?limit=10", nil)

	if got := redactTicket(request); got != request {
		t.Error("redactTicket() should return the original request when there is no ticket")
	}
}
