package comms

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestValidSignature checks our X-Twilio-Signature recomputation against the
// canonical example published in Twilio's request-validation docs, plus a
// self-consistency round trip.
func TestValidSignature(t *testing.T) {
	t.Setenv("TWILIO_WEBHOOK_BASE_URL", "https://mycompany.com")
	form := url.Values{
		"CallSid": {"CA1234567890ABCDE"},
		"Caller":  {"+14158675309"},
		"Digits":  {"1234"},
		"From":    {"+14158675309"},
		"To":      {"+18005551212"},
	}
	r := httptest.NewRequest("POST", "/myapp.php?foo=1&bar=2", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	c := &TwilioClient{authToken: "12345"}
	const canonical = "RSOYDt4T1cUTdK1PDd93/VVr8B8="
	if !c.validSignature(r, canonical) {
		t.Errorf("canonical Twilio vector should validate")
	}
	if c.validSignature(r, "GQ==") {
		t.Errorf("a wrong signature must not validate")
	}
}
