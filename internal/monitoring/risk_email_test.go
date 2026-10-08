package monitoring

import (
	"net/mail"
	"strings"
	"testing"
)

func TestRiskEmailValidate(t *testing.T) {
	ok := riskEmailRequest{To: "a@x.com", CC: []string{"b@x.com"}, Subject: " s ", Body: "b"}
	to, cc, err := ok.validate()
	if err != nil || to != "a@x.com" || len(cc) != 1 || ok.Subject != "s" {
		t.Fatalf("valid request rejected: %v %v %v", to, cc, err)
	}
	bad := []riskEmailRequest{
		{To: "nope", Subject: "s", Body: "b"},
		{To: "a@x.com", Subject: "s\r\nBcc: z@x.com", Body: "b"},
		{To: "a@x.com", CC: []string{"Evil <e@x.com>"}, Subject: "s", Body: "b"},
		{To: "a@x.com", Subject: "", Body: "b"},
		{To: "a@x.com", CC: make([]string, 11), Subject: "s", Body: "b"},
	}
	for i, r := range bad {
		if _, _, err := r.validate(); err == nil {
			t.Fatalf("case %d should be rejected", i)
		}
	}
}

func TestBuildRiskEmailHeaders(t *testing.T) {
	msg := string(buildRiskEmail(mail.Address{Name: "QA", Address: "q@x.com"}, "a@x.com", []string{"b@x.com", "c@x.com"}, "Risiko", "baris1\nbaris2"))
	for _, want := range []string{"To: a@x.com", "Cc: b@x.com, c@x.com", "\r\n\r\nbaris1\r\nbaris2"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in %q", want, msg)
		}
	}
}
