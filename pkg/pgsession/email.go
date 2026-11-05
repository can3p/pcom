package pgsession

import "github.com/can3p/pcom/pkg/emailaddr"

// NormalizeEmail is the form an email address is stored and looked up in.
func NormalizeEmail(email string) string {
	return emailaddr.Normalize(email)
}

// CanonicalEmail is the mailbox an address delivers to, stored as
// users.email_canonical so that signup can allow one account per mailbox
// (emailaddr.Parse).
func CanonicalEmail(email string) string {
	return emailaddr.Parse(email).Canonical
}
