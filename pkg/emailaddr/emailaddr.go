// Package emailaddr is the one place that classifies email addresses: the
// form one is stored in, the mailbox it delivers to (the key that allows one
// account per mailbox), whether its domain is a disposable or a relay
// service, and whether the domain can receive mail at all.
//
// It does not decide whether an address is well-formed: that is
// validation.EmailRE, checked before. The classification comes from
// github.com/can3p/anti-disposable-email, whose parser is stricter than
// EmailRE (it refuses, for one, the .test addresses tests and development
// use); an address it can't parse is neither disposable nor a relay, and
// gets pcom's own canonical rule.
package emailaddr

import (
	"strings"

	disposable "github.com/can3p/anti-disposable-email"
)

// Address is what pcom knows about an email address from its text alone.
type Address struct {
	// Stored is the form the address is stored and looked up in (Normalize).
	Stored string
	// Canonical is the mailbox the address delivers to, stored as
	// users.email_canonical. It is a uniqueness key, not an address to send
	// mail to.
	Canonical string
	// Disposable is true for a throwaway mailbox service, its subdomains
	// included.
	Disposable bool
	// Relay is true for a forwarding or alias service (duck.com,
	// relay.firefox.com), usually a real person protecting their privacy.
	Relay bool
}

// Normalize is the form an email address is stored and looked up in: trimmed
// and lower-cased.
func Normalize(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// Parse classifies an address. Canonical is the library's canonical form,
// which knows the rules of the big providers (Gmail ignores dots,
// googlemail.com is gmail.com, Yahoo's -keyword), with pcom's rule on top that
// a +tag is dropped on every domain. An address the library can't parse gets
// only pcom's rule.
func Parse(address string) Address {
	a := Address{Stored: Normalize(address)}

	p, err := disposable.ParseEmail(a.Stored)
	if err != nil {
		a.Canonical = dropTag(a.Stored)
		return a
	}

	a.Canonical = dropTag(p.Canonical)
	a.Disposable = p.Disposable
	a.Relay = p.Relay

	return a
}

// dropTag removes a +tag from the local part of an address.
func dropTag(address string) string {
	at := strings.LastIndex(address, "@")
	if at < 0 {
		return address
	}

	local, _, _ := strings.Cut(address[:at], "+")

	return local + address[at:]
}
