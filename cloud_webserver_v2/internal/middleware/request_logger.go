package hytech_middleware

import (
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/go-chi/chi/v5/middleware"
)

// TicketQueryParam is the query parameter CAS uses to hand our callback a service ticket.
const TicketQueryParam = "ticket"

// NewRedactingLogFormatter returns a chi log formatter that behaves like the default one
// but never writes a service ticket to the log. CAS tickets are single-use credentials, so
// they must not be persisted anywhere, logs included.
func NewRedactingLogFormatter() middleware.LogFormatter {
	return &redactingLogFormatter{
		base: &middleware.DefaultLogFormatter{
			Logger:  log.New(os.Stdout, "", log.LstdFlags),
			NoColor: runtime.GOOS == "windows",
		},
	}
}

type redactingLogFormatter struct {
	base middleware.LogFormatter
}

func (f *redactingLogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return f.base.NewLogEntry(redactTicket(r))
}

// redactTicket returns a copy of the request whose logged URI hides the ticket. The
// original request is left untouched so handlers can still read the real ticket.
func redactTicket(r *http.Request) *http.Request {
	if r.URL.Query().Get(TicketQueryParam) == "" {
		return r
	}

	redacted := r.Clone(r.Context())
	query := redacted.URL.Query()
	query.Set(TicketQueryParam, "[redacted]")
	redacted.URL.RawQuery = query.Encode()
	// chi's default formatter logs RequestURI, not the parsed URL.
	redacted.RequestURI = redacted.URL.RequestURI()
	return redacted
}
