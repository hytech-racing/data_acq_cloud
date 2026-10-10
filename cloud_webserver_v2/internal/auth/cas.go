package auth

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Georgia Tech runs CAS 3.0 at sso.gatech.edu. We validate service tickets against
// the /p3/serviceValidate endpoint, unlike the CAS 2.0 endpoint it also returns
// user attributes such as mail and displayName.
const (
	DefaultCASBaseURL   = "https://sso.gatech.edu/cas"
	casLoginPath        = "/login"
	casLogoutPath       = "/logout"
	casValidatePath     = "/p3/serviceValidate"
	maxCASResponseBytes = 1 << 20 // A CAS response is tiny, so this only guards against a misbehaving server.
)

// ErrAuthenticationFailed is returned when CAS answers without a user and without
// an explicit authenticationFailure, which means the response could not be trusted.
var ErrAuthenticationFailed = errors.New("cas authentication failed")

// ValidationError describes an authenticationFailure returned by the CAS server.
// The Code is one of CAS's error codes, for example INVALID_TICKET or INVALID_SERVICE.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Message == "" {
		return "cas validation failed: " + e.Code
	}
	return fmt.Sprintf("cas validation failed: %s: %s", e.Code, e.Message)
}

// User is an authenticated Georgia Tech user as returned by a successful CAS validation.
type User struct {
	Username   string            `json:"username"`
	Attributes map[string]string `json:"attributes"`
}

// Attribute returns the value of a CAS released attribute, for example "mail" or "displayName".
func (u *User) Attribute(name string) string {
	return u.Attributes[name]
}

// CASClient talks to the Georgia Tech CAS server for a single registered service.
// The service URL is held in one place so the value sent to /login always matches
// the value sent to /p3/serviceValidate; a mismatch is the usual cause of INVALID_TICKET.
type CASClient struct {
	baseURL    string
	serviceURL string
	httpClient *http.Client
}

func NewCASClient(baseURL, serviceURL string) *CASClient {
	return &CASClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		serviceURL: serviceURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ServiceURL returns the callback URL this client authenticates against.
func (c *CASClient) ServiceURL() string {
	return c.serviceURL
}

// LoginURL builds the CAS login URL the browser should be redirected to.
func (c *CASClient) LoginURL() string {
	return c.baseURL + casLoginPath + "?service=" + url.QueryEscape(c.serviceURL)
}

// LogoutURL builds the CAS logout URL, optionally asking CAS to send the browser
// back to redirectURL once its own session has been destroyed.
func (c *CASClient) LogoutURL(redirectURL string) string {
	logoutURL := c.baseURL + casLogoutPath
	if redirectURL == "" {
		return logoutURL
	}
	return logoutURL + "?service=" + url.QueryEscape(redirectURL)
}

// Validate exchanges a one-time service ticket for the authenticated user.
// Tickets expire within seconds and can only be used once, so this must be called
// immediately when the callback is received.
func (c *CASClient) Validate(ctx context.Context, ticket string) (*User, error) {
	if strings.TrimSpace(ticket) == "" {
		return nil, &ValidationError{Code: "INVALID_TICKET", Message: "missing service ticket"}
	}

	validateURL := c.baseURL + casValidatePath +
		"?ticket=" + url.QueryEscape(ticket) +
		"&service=" + url.QueryEscape(c.serviceURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validateURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build cas validation request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach cas validation endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCASResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read cas validation response: %w", err)
	}

	return parseCASResponse(body)
}

type serviceResponse struct {
	XMLName               xml.Name               `xml:"serviceResponse"`
	AuthenticationSuccess *authenticationSuccess `xml:"authenticationSuccess"`
	AuthenticationFailure *authenticationFailure `xml:"authenticationFailure"`
}

type authenticationSuccess struct {
	User       string        `xml:"user"`
	Attributes casAttributes `xml:"attributes"`
}

type authenticationFailure struct {
	Code    string `xml:"code,attr"`
	Message string `xml:",chardata"`
}

type casAttributes struct {
	Attributes []casAttribute `xml:",any"`
}

// casAttribute covers both shapes CAS servers use for released attributes:
// nested elements (<cas:mail>...</cas:mail>) and name/value pairs (<cas:attribute name="mail" value="..."/>).
type casAttribute struct {
	XMLName xml.Name
	Name    string `xml:"name,attr"`
	Value   string `xml:"value,attr"`
	Text    string `xml:",chardata"`
}

func parseCASResponse(body []byte) (*User, error) {
	var parsed serviceResponse
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse cas response: %w", err)
	}

	if parsed.AuthenticationFailure != nil {
		return nil, &ValidationError{
			Code:    parsed.AuthenticationFailure.Code,
			Message: strings.TrimSpace(parsed.AuthenticationFailure.Message),
		}
	}

	if parsed.AuthenticationSuccess == nil || strings.TrimSpace(parsed.AuthenticationSuccess.User) == "" {
		return nil, ErrAuthenticationFailed
	}

	user := &User{
		Username:   strings.TrimSpace(parsed.AuthenticationSuccess.User),
		Attributes: make(map[string]string),
	}

	for _, attribute := range parsed.AuthenticationSuccess.Attributes.Attributes {
		name := attribute.Name
		if name == "" {
			name = attribute.XMLName.Local
		}

		value := attribute.Value
		if value == "" {
			value = strings.TrimSpace(attribute.Text)
		}

		if name != "" && value != "" {
			user.Attributes[name] = value
		}
	}

	return user, nil
}
