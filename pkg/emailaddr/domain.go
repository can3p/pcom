package emailaddr

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/logging"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/zap"
)

// The settings' values when the Checker is built without the options; they
// are also the configuration's defaults.
const (
	// DefaultMXTimeout is how long CheckDomain waits for DNS when built
	// without WithTimeout.
	DefaultMXTimeout = 2 * time.Second
	// DefaultCacheTTL is how long CheckDomain remembers a domain's answer
	// when built without WithCacheTTL.
	DefaultCacheTTL = time.Hour
	// DefaultCacheSize is how many domains' answers CheckDomain remembers
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
// answers of lookups that got one for a while, so a domain asked about again
// costs no DNS traffic.
type Checker struct {
	resolver        Resolver
	timeout         time.Duration
	allowDisposable bool
	cacheTTL        time.Duration
	cacheSize       int
	now             func() time.Time
	// cache is nil when caching is off.
	cache *lru.Cache[string, answer]
}

// answer is a cached CheckDomain result: nil or ErrCantReceiveMail.
type answer struct {
	err     error
	expires time.Time
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

// WithCacheTTL sets how long a domain's answer is remembered; 0 turns the
// cache off. Only definite answers are remembered: a failed or timed-out
// lookup, which lets the address through, is asked again next time.
func WithCacheTTL(d time.Duration) Option {
	return func(c *Checker) { c.cacheTTL = d }
}

// WithCacheSize sets how many domains' answers are remembered; past it the
// least recently used is forgotten. 0 turns the cache off.
func WithCacheSize(n int) Option {
	return func(c *Checker) { c.cacheSize = n }
}

// WithClock replaces the clock the cache's answers expire by, for tests.
func WithClock(now func() time.Time) Option {
	return func(c *Checker) { c.now = now }
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
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.cacheTTL > 0 && c.cacheSize > 0 {
		// New fails only for a size below one.
		c.cache, _ = lru.New[string, answer](c.cacheSize)
	}

	return c
}

// CheckDomain returns ErrCantReceiveMail when the domain doesn't exist, has a
// null MX (RFC 7505: a single MX of "."), or has neither an MX nor an A or
// AAAA record. Any other DNS failure, a timeout included, lets the domain
// through and is logged: an outage on our side must not refuse addresses.
// The answer of a lookup that got one is remembered for the cache's TTL.
func (c *Checker) CheckDomain(ctx context.Context, domain string) error {
	if c.timeout <= 0 {
		return nil
	}

	// A fully qualified name, so the resolver doesn't try the search domains.
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".") + "."

	if c.cache != nil {
		if a, ok := c.cache.Get(name); ok {
			if c.now().Before(a.expires) {
				return a.err
			}

			c.cache.Remove(name)
		}
	}

	definite, err := c.lookup(ctx, domain, name)
	if definite && c.cache != nil {
		c.cache.Add(name, answer{err: err, expires: c.now().Add(c.cacheTTL)})
	}

	return err
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
