package validation

import (
	"regexp"
)

var EmailRE *regexp.Regexp = regexp.MustCompile(`(?P<name>[a-zA-Z0-9.!#$%&'*+/=?^_ \x60{|}~-]+)@(?P<domain>[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*)$`)
var TestEmailRE *regexp.Regexp = regexp.MustCompile(`dpetroff(\+[^@]+)?@gmail.com`)
var AttributionRE *regexp.Regexp = regexp.MustCompile(`^[a-z_]+$`)
