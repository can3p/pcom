package emailaddr

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/logging"
	"go.uber.org/zap"
)

// DefaultMXTimeout is how long CheckDomain waits for DNS when built without
// WithTimeout.
const DefaultMXTimeout = 2 * time.Second

// ErrCantReceiveMail is CheckDomain's answer for a domain that can't receive
// mail. UndeliverableMessage is the field message to show for it.
var ErrCantReceiveMail = errors.New("the domain can't receive mail")

// UndeliverableMessage is what a form says about an address whose domain
// can't receive mail.
const UndeliverableMessage = "This address can't receive mail"

// Resolver is the part of *net.Resolver that CheckDomain uses.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Checker looks up whether a domain can receive mail.
type Checker struct {
	resolver        Resolver
	timeout         time.Duration
	allowDisposable bool
}

// Option configures a Checker.
type Option func(*Checker)

// WithResolver replaces net.DefaultResolver.
func WithResolver(r Resolver) Option {
	return func(c *Checker) { c.resolver = r }
}

// WithTimeout bounds the DNS lookups of one CheckDomain; 0 turns the check
// off, so that every domain passes.
func WithTimeout(d time.Duration) Option {
	return func(c *Checker) { c.timeout = d }
}

// WithAllowDisposable makes AllowsDisposable true, so that a caller accepts
// throwaway addresses: for local development, where every address is made up.
func WithAllowDisposable(allow bool) Option {
	return func(c *Checker) { c.allowDisposable = allow }
}

// AllowsDisposable reports whether a disposable address is accepted.
func (c *Checker) AllowsDisposable() bool { return c.allowDisposable }

// NewChecker builds a Checker on net.DefaultResolver with DefaultMXTimeout.
func NewChecker(opts ...Option) *Checker {
	c := &Checker{resolver: net.DefaultResolver, timeout: DefaultMXTimeout}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CheckDomain returns ErrCantReceiveMail when the domain doesn't exist, has a
// null MX (RFC 7505: a single MX of "."), or has neither an MX nor an A or
// AAAA record. Any other DNS failure, a timeout included, lets the domain
// through and is logged: an outage on our side must not refuse addresses.
func (c *Checker) CheckDomain(ctx context.Context, domain string) error {
	if c.timeout <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// A fully qualified name, so the resolver doesn't try the search domains.
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".") + "."

	mx, err := c.resolver.LookupMX(ctx, name)
	switch {
	case err == nil && len(mx) == 1 && (mx[0].Host == "." || mx[0].Host == ""):
		return ErrCantReceiveMail
	case err == nil && len(mx) > 0:
		return nil
	case err != nil && !notFound(err):
		return letThrough(ctx, domain, "mx", err)
	}

	// No MX: mail goes to the domain's own address (RFC 5321, 5.1), and a
	// domain that doesn't exist has none either.
	ips, err := c.resolver.LookupIPAddr(ctx, name)
	switch {
	case err == nil && len(ips) > 0:
		return nil
	case err == nil || notFound(err):
		return ErrCantReceiveMail
	default:
		return letThrough(ctx, domain, "address", err)
	}
}

// notFound is the resolver's answer for a name that doesn't exist or has no
// records of the type asked for; Go's resolver doesn't tell the two apart.
func notFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func letThrough(ctx context.Context, domain, lookup string, err error) error {
	logging.FromContext(ctx).Warn("email domain check failed, letting the address through",
		zap.String("domain", domain), zap.String("lookup", lookup), zap.Error(err))

	return nil
}
