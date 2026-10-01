package pgsession

import "strings"

// NormalizeEmail is the form an email address is stored and looked up in.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
