package pgsession

import "strings"

// NormalizeEmail is the form an email address is stored and looked up in.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// CanonicalEmail is the mailbox an address delivers to, stored as
// users.email_canonical so that signup can allow one account per mailbox: the
// normalized address without a +tag, and for Gmail without the dots it
// ignores and with googlemail.com as gmail.com.
func CanonicalEmail(email string) string {
	email = NormalizeEmail(email)

	at := strings.LastIndex(email, "@")
	if at < 0 {
		return email
	}

	local, domain := email[:at], email[at+1:]
	local, _, _ = strings.Cut(local, "+")

	if domain == "gmail.com" || domain == "googlemail.com" {
		return strings.ReplaceAll(local, ".", "") + "@gmail.com"
	}

	return local + "@" + domain
}
