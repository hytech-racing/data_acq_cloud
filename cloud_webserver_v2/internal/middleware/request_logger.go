package hytech_middleware

import (
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/go-chi/chi/v5/middleware"
)

// AuthorizationCodeQueryParam is the query parameter the OIDC provider uses to hand our
// callback an authorization code.
const AuthorizationCodeQueryParam = "code"

// NewRedactingLogFormatter returns a chi log formatter that behaves like the default one
// but never writes an authorization code to the log. Authorization codes are single-use
// credentials, so they must not be persisted anywhere, logs included.
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
	return f.base.NewLogEntry(redactAuthorizationCode(r))
}

// redactAuthorizationCode returns a copy of the request whose logged URI hides the
// authorization code. The original request is left untouched so handlers can still read
// the real code.
func redactAuthorizationCode(r *http.Request) *http.Request {
	if r.URL.Query().Get(AuthorizationCodeQueryParam) == "" {
		return r
	}

	redacted := r.Clone(r.Context())
	query := redacted.URL.Query()
	query.Set(AuthorizationCodeQueryParam, "[redacted]")
	redacted.URL.RawQuery = query.Encode()
	// chi's default formatter logs RequestURI, not the parsed URL.
	redacted.RequestURI = redacted.URL.RequestURI()
	return redacted
}
