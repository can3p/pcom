package emailaddr

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/logging"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// The settings' values when the Checker is built without the options; they
// are also the configuration's defaults.
const (
	// DefaultMXTimeout is how long CheckDomain waits for DNS when built
	// without WithTimeout.
	DefaultMXTimeout = 2 * time.Second
	// DefaultCacheTTL is how long CheckDomain remembers an accepted domain
	// when built without WithCacheTTL.
	DefaultCacheTTL = time.Hour
	// DefaultCacheSize is how many accepted domains CheckDomain remembers
	// when built without WithCacheSize.
	DefaultCacheSize = 10000
)

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

// Checker looks up whether a domain can receive mail. It remembers the
// domains it accepted for a while, so a domain asked about again costs no DNS
// traffic; a refusal is never remembered, so a domain fixed since is accepted
// on the next try. Concurrent checks of one domain share one lookup.
type Checker struct {
	resolver        Resolver
	timeout         time.Duration
	allowDisposable bool
	cacheTTL        time.Duration
	cacheSize       int
	// cache holds the accepted domains; nil when caching is off.
	cache   *expirable.LRU[string, struct{}]
	lookups singleflight.Group
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

// WithCacheTTL sets how long an accepted domain is remembered; 0 turns the
// cache off. Only domains a lookup accepted are remembered: a refused domain,
// or a failed or timed-out lookup, is asked again next time.
func WithCacheTTL(d time.Duration) Option {
	return func(c *Checker) { c.cacheTTL = d }
}

// WithCacheSize sets how many accepted domains are remembered; past it the
// least recently used is forgotten. 0 turns the cache off.
func WithCacheSize(n int) Option {
	return func(c *Checker) { c.cacheSize = n }
}

// WithAllowDisposable makes AllowsDisposable true, so that a caller accepts
// throwaway addresses: for local development, where every address is made up.
func WithAllowDisposable(allow bool) Option {
	return func(c *Checker) { c.allowDisposable = allow }
}

// AllowsDisposable reports whether a disposable address is accepted.
func (c *Checker) AllowsDisposable() bool { return c.allowDisposable }

// NewChecker builds a Checker on net.DefaultResolver with DefaultMXTimeout,
// DefaultCacheTTL and DefaultCacheSize.
func NewChecker(opts ...Option) *Checker {
	c := &Checker{
		resolver:  net.DefaultResolver,
		timeout:   DefaultMXTimeout,
		cacheTTL:  DefaultCacheTTL,
		cacheSize: DefaultCacheSize,
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.cacheTTL > 0 && c.cacheSize > 0 {
		c.cache = expirable.NewLRU[string, struct{}](c.cacheSize, nil, c.cacheTTL)
	}

	return c
}

// NeedsLookup reports whether CheckDomain would ask DNS about the domain now:
// the check is on and the domain isn't remembered as accepted. A caller that
// budgets lookups asks this first.
func (c *Checker) NeedsLookup(domain string) bool {
	return c.timeout > 0 && !c.cached(fqdn(domain))
}

// CheckDomain returns ErrCantReceiveMail when the domain doesn't exist, has a
// null MX (RFC 7505: a single MX of "."), or has neither an MX nor an A or
// AAAA record. Any other DNS failure, a timeout included, lets the domain
// through and is logged: an outage on our side must not refuse addresses.
// An accepted domain is remembered for the cache's TTL. looked reports
// whether this call asked DNS itself: false for a remembered domain, a check
// that is off, and a call that shared a concurrent call's lookup.
func (c *Checker) CheckDomain(ctx context.Context, domain string) (looked bool, err error) {
	if c.timeout <= 0 {
		return false, nil
	}

	name := fqdn(domain)
	if c.cached(name) {
		return false, nil
	}

	_, err, _ = c.lookups.Do(name, func() (any, error) {
		looked = true

		definite, err := c.lookup(ctx, domain, name)
		if definite && err == nil && c.cache != nil {
			c.cache.Add(name, struct{}{})
		}

		return nil, err
	})

	return looked, err
}

// fqdn is the domain as a fully qualified name, so the resolver doesn't try
// the search domains.
func fqdn(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".") + "."
}

// cached reports whether the fully qualified name is remembered as accepted.
func (c *Checker) cached(name string) bool {
	if c.cache == nil {
		return false
	}

	_, ok := c.cache.Get(name)

	return ok
}

// lookup asks DNS whether the fully qualified name can receive mail. definite
// is false when the lookup failed and the domain was let through.
func (c *Checker) lookup(ctx context.Context, domain, name string) (definite bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	mx, err := c.resolver.LookupMX(ctx, name)
	switch {
	case err == nil && len(mx) == 1 && (mx[0].Host == "." || mx[0].Host == ""):
		return true, ErrCantReceiveMail
	case err == nil && len(mx) > 0:
		return true, nil
	case err != nil && !notFound(err):
		return false, letThrough(ctx, domain, "mx", err)
	}

	// No MX: mail goes to the domain's own address (RFC 5321, 5.1), and a
	// domain that doesn't exist has none either.
	ips, err := c.resolver.LookupIPAddr(ctx, name)
	switch {
	case err == nil && len(ips) > 0:
		return true, nil
	case err == nil || notFound(err):
		return true, ErrCantReceiveMail
	default:
		return false, letThrough(ctx, domain, "address", err)
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
