// Package comms holds the outbound messaging clients (Twilio SMS, SMTP
// email) shared by SMS intake and vendor outreach. Credentials are
// platform-level (env); the per-org "from" identity is passed per call.
package comms

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

// TwilioClient is a thin REST client for the Twilio Messages API. Ported
// from ra-avm's sms/service/twilio.go, with the from-number made a per-call
// argument so each org can send from its own number.
type TwilioClient struct {
	accountSID string
	authToken  string
	httpClient *http.Client
}

func NewTwilioClient(accountSID, authToken string) *TwilioClient {
	return &TwilioClient{
		accountSID: accountSID,
		authToken:  authToken,
		httpClient: &http.Client{},
	}
}

func (c *TwilioClient) Configured() bool {
	return c != nil && c.accountSID != "" && c.authToken != ""
}

// SendSMS sends one SMS and returns the Twilio message SID.
func (c *TwilioClient) SendSMS(ctx context.Context, from, to, body string) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("twilio client not configured")
	}
	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", c.accountSID)

	data := url.Values{}
	data.Set("To", to)
	data.Set("From", from)
	data.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.accountSID, c.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send SMS: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("twilio error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		SID string `json:"sid"`
	}
	_ = json.Unmarshal(respBody, &parsed)
	return parsed.SID, nil
}

// ValidateWebhook authenticates an inbound Twilio webhook. It always requires
// the AccountSid to match, and — when an auth token is configured — verifies
// the full X-Twilio-Signature HMAC (Twilio's scheme: HMAC-SHA1 over the request
// URL followed by the POST params sorted by key, keyed by the auth token).
//
// Local escape hatches: with no auth token (dev / OUTREACH_SIMULATE we can't
// compute the HMAC) or with TWILIO_SKIP_SIGNATURE_VALIDATION set, it falls back
// to the AccountSid check so the e2e harness works without real signatures.
// Production (Render) sets neither, so the signature is enforced.
func (c *TwilioClient) ValidateWebhook(r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		return false
	}
	if r.FormValue("AccountSid") != c.accountSID {
		return false
	}
	if c.authToken == "" || isTruthy(os.Getenv("TWILIO_SKIP_SIGNATURE_VALIDATION")) {
		return true
	}
	sig := r.Header.Get("X-Twilio-Signature")
	if sig == "" {
		return false
	}
	return c.validSignature(r, sig)
}

// validSignature recomputes Twilio's HMAC-SHA1 signature and compares it to the
// provided X-Twilio-Signature in constant time.
func (c *TwilioClient) validSignature(r *http.Request, provided string) bool {
	var b strings.Builder
	b.WriteString(c.webhookURL(r))

	keys := make([]string, 0, len(r.PostForm))
	for k := range r.PostForm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(r.PostForm.Get(k))
	}

	mac := hmac.New(sha1.New, []byte(c.authToken))
	mac.Write([]byte(b.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(provided))
}

// webhookURL reconstructs the public URL Twilio signed. Behind a TLS-terminating
// proxy (Render) the internal request looks like plain HTTP, so trust the
// X-Forwarded-* headers; TWILIO_WEBHOOK_BASE_URL overrides both if a proxy
// rewrites host/proto in a way that breaks the match.
func (c *TwilioClient) webhookURL(r *http.Request) string {
	if base := os.Getenv("TWILIO_WEBHOOK_BASE_URL"); base != "" {
		return strings.TrimRight(base, "/") + r.URL.RequestURI()
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host + r.URL.RequestURI()
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

var nonDigit = regexp.MustCompile(`[^0-9]`)

// NormalizePhone strips non-digits and returns the last 10 digits, so E.164
// (+12035551234) and human formats ((203) 555-1234) compare equal.
func NormalizePhone(phone string) string {
	digits := nonDigit.ReplaceAllString(phone, "")
	if len(digits) > 10 {
		digits = digits[len(digits)-10:]
	}
	return digits
}

// SMTPClient sends plain-text email through a standard SMTP relay.
type SMTPClient struct {
	host     string
	port     string
	username string
	password string
}

func NewSMTPClient(host, port, username, password string) *SMTPClient {
	return &SMTPClient{host: host, port: port, username: username, password: password}
}

func (c *SMTPClient) Configured() bool {
	return c != nil && c.host != ""
}

func (c *SMTPClient) SendEmail(ctx context.Context, from, to, subject, body string) error {
	if !c.Configured() {
		return fmt.Errorf("smtp client not configured")
	}
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if c.username != "" {
		auth = smtp.PlainAuth("", c.username, c.password, c.host)
	}
	return smtp.SendMail(c.host+":"+c.port, auth, from, []string{to}, []byte(msg))
}
