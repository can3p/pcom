package emailaddr_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/stretchr/testify/require"
)

func TestParse_Canonical(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		// Gmail: +tag and dots ignored, googlemail.com is gmail.com
		"First.Last+x@gmail.com":    "firstlast@gmail.com",
		"first.last@googlemail.com": "firstlast@gmail.com",
		// iCloud and Proton: one namespace across their domains
		"john+news@me.com":      "john@icloud.com",
		"john@mac.com":          "john@icloud.com",
		"john+a@icloud.com":     "john@icloud.com",
		"jane+x@protonmail.com": "jane@proton.me",
		"jane@protonmail.ch":    "jane@proton.me",
		"jane@pm.me":            "jane@proton.me",
		"jane+y@proton.me":      "jane@proton.me",
		// Microsoft: each domain its own namespace
		"bob+shop@outlook.com":   "bob@outlook.com",
		"bob+shop@hotmail.co.uk": "bob@hotmail.co.uk",
		"bob@live.com":           "bob@live.com",
		"bob+a@msn.com":          "bob@msn.com",
		// Yahoo: -keyword
		"amy-shop@yahoo.com":      "amy@yahoo.com",
		"amy-shop@ymail.com":      "amy@ymail.com",
		"amy-shop@rocketmail.com": "amy@rocketmail.com",
		// Fastmail: +tag, and anything@user.fastmail.com
		"kim+x@fastmail.com":        "kim@fastmail.com",
		"kim+x@fastmail.fm":         "kim@fastmail.fm",
		"anything@kim.fastmail.com": "kim@fastmail.com",
		// the others with a +tag
		"lee+x@hey.com":      "lee@hey.com",
		"lee+x@yandex.ru":    "lee@yandex.ru",
		"lee+x@zohomail.com": "lee@zohomail.com",
		"lee+x@mailbox.org":  "lee@mailbox.org",
		"lee+x@posteo.de":    "lee@posteo.de",
		// pcom drops a +tag on every domain; dots count outside Gmail
		" User+promo@Pcom.Dev ":   "user@pcom.dev",
		"user+a+b@pcom.dev":       "user@pcom.dev",
		"first.last@pcom.dev":     "first.last@pcom.dev",
		"user@gmail.com.pcom.dev": "user@gmail.com.pcom.dev",
		"+tag@pcom.dev":           "@pcom.dev",
		// the library can't parse these: pcom's rule alone
		"User+tag@Example.test": "user@example.test",
		"not-an-address":        "not-an-address",
	} {
		require.Equal(t, want, emailaddr.Parse(in).Canonical, in)
	}
}

func TestParse_Classification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		address string
		want    emailaddr.Address
	}{
		{" Jane@Pcom.Dev ", emailaddr.Address{Stored: "jane@pcom.dev", Canonical: "jane@pcom.dev"}},
		{"jane@example.test", emailaddr.Address{Stored: "jane@example.test", Canonical: "jane@example.test"}},
		{"Test@Mailinator.com", emailaddr.Address{Stored: "test@mailinator.com", Canonical: "test@mailinator.com", Disposable: true}},
		{"test@foo.mailinator.com", emailaddr.Address{Stored: "test@foo.mailinator.com", Canonical: "test@foo.mailinator.com", Disposable: true}},
		{"user@example.com", emailaddr.Address{Stored: "user@example.com", Canonical: "user@example.com", Disposable: true}},
		{"me+x@duck.com", emailaddr.Address{Stored: "me+x@duck.com", Canonical: "me@duck.com", Relay: true}},
		{"me@relay.firefox.com", emailaddr.Address{Stored: "me@relay.firefox.com", Canonical: "me@relay.firefox.com", Relay: true}},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, emailaddr.Parse(tt.address))
		})
	}
}
