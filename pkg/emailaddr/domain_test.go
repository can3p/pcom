package emailaddr_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/testutil/testlog"
	"github.com/stretchr/testify/require"
)

// fakeResolver answers for one domain; a nil answer with a nil error is no
// records.
type fakeResolver struct {
	mx     []*net.MX
	mxErr  error
	ips    []net.IPAddr
	ipErr  error
	block  bool
	lookup []string
}

func (f *fakeResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	f.lookup = append(f.lookup, "mx "+name)
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	return f.mx, f.mxErr
}

func (f *fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	f.lookup = append(f.lookup, "ip "+host)
	return f.ips, f.ipErr
}

func TestCheckDomain(t *testing.T) {
	t.Parallel()

	notFound := &net.DNSError{Err: "no such host", Name: "mail.test.", IsNotFound: true}
	servfail := &net.DNSError{Err: "server misbehaving", Name: "mail.test.", IsTemporary: true}
	ip := []net.IPAddr{{IP: net.IPv4(192, 0, 2, 1)}}

	tests := []struct {
		name     string
		resolver *fakeResolver
		want     error
		logged   bool
	}{
		{"MX records", &fakeResolver{mx: []*net.MX{{Host: "mx.mail.test.", Pref: 10}}}, nil, false},
		{"no such domain", &fakeResolver{mxErr: notFound, ipErr: notFound}, emailaddr.ErrCantReceiveMail, false},
		{"null MX", &fakeResolver{mx: []*net.MX{{Host: ".", Pref: 0}}, ips: ip}, emailaddr.ErrCantReceiveMail, false},
		{"no MX, an A record", &fakeResolver{mxErr: notFound, ips: ip}, nil, false},
		{"no MX, no A", &fakeResolver{ipErr: notFound}, emailaddr.ErrCantReceiveMail, false},
		{"SERVFAIL", &fakeResolver{mxErr: servfail}, nil, true},
		{"SERVFAIL on the A lookup", &fakeResolver{mxErr: notFound, ipErr: servfail}, nil, true},
		{"timeout", &fakeResolver{block: true}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, logs := testlog.Context(context.Background())
			checker := emailaddr.NewChecker(emailaddr.WithResolver(tt.resolver), emailaddr.WithTimeout(50*time.Millisecond))

			looked, err := checker.CheckDomain(ctx, "Mail.test")
			require.True(t, looked)
			if tt.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.want)
			}
			require.Equal(t, "mx mail.test.", tt.resolver.lookup[0])
			require.Equal(t, tt.logged, logs.Len() > 0)
		})
	}
}

func TestCheckDomain_ZeroTimeoutIsOff(t *testing.T) {
	t.Parallel()

	r := &fakeResolver{mxErr: errors.New("must not be called")}
	checker := emailaddr.NewChecker(emailaddr.WithResolver(r), emailaddr.WithTimeout(0))
	looked, err := checker.CheckDomain(context.Background(), "mail.test")

	require.NoError(t, err)
	require.False(t, looked)
	require.False(t, checker.NeedsLookup("mail.test"))
	require.Empty(t, r.lookup)
}

// Only an accepted domain is remembered: a refusal, or a lookup that failed
// and let the domain through, is asked again.
func TestCheckDomain_Cache(t *testing.T) {
	t.Parallel()

	notFound := &net.DNSError{Err: "no such host", Name: "mail.test.", IsNotFound: true}
	servfail := &net.DNSError{Err: "server misbehaving", Name: "mail.test.", IsTemporary: true}
	mx := []*net.MX{{Host: "mx.mail.test.", Pref: 10}}

	tests := []struct {
		name     string
		resolver *fakeResolver
		want     error
		cached   bool
	}{
		{"accepted", &fakeResolver{mx: mx}, nil, true},
		{"refused", &fakeResolver{mxErr: notFound, ipErr: notFound}, emailaddr.ErrCantReceiveMail, false},
		{"SERVFAIL", &fakeResolver{mxErr: servfail}, nil, false},
		{"timeout", &fakeResolver{block: true}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checker := emailaddr.NewChecker(emailaddr.WithResolver(tt.resolver), emailaddr.WithTimeout(50*time.Millisecond))
			check := func(wantLooked bool) {
				t.Helper()

				require.Equal(t, wantLooked, checker.NeedsLookup("Mail.test"))

				looked, err := checker.CheckDomain(context.Background(), "Mail.test")
				require.Equal(t, wantLooked, looked)
				if tt.want == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, tt.want)
				}
			}

			check(true)
			first := len(tt.resolver.lookup)

			check(!tt.cached)

			if tt.cached {
				require.Len(t, tt.resolver.lookup, first, "an accepted domain is remembered")
			} else {
				require.Len(t, tt.resolver.lookup, 2*first, "only an accepted domain is remembered")
			}
		})
	}
}

// A refused domain that has mail since is accepted on the next check.
func TestCheckDomain_RefusalNotCached(t *testing.T) {
	t.Parallel()

	r := &fakeResolver{mxErr: &net.DNSError{Err: "no such host", Name: "mail.test.", IsNotFound: true}, ipErr: &net.DNSError{IsNotFound: true}}
	checker := emailaddr.NewChecker(emailaddr.WithResolver(r))

	_, err := checker.CheckDomain(context.Background(), "mail.test")
	require.ErrorIs(t, err, emailaddr.ErrCantReceiveMail)

	r.mx, r.mxErr = []*net.MX{{Host: "mx.mail.test.", Pref: 10}}, nil
	looked, err := checker.CheckDomain(context.Background(), "mail.test")
	require.NoError(t, err)
	require.True(t, looked)
}

func TestCheckDomain_CacheExpires(t *testing.T) {
	t.Parallel()

	r := byName{}
	ttl := 300 * time.Millisecond // wide enough that two calls in a row fall inside it on a slow machine
	checker := emailaddr.NewChecker(emailaddr.WithResolver(r), emailaddr.WithCacheTTL(ttl))

	for _, wait := range []time.Duration{0, 0, 2 * ttl} {
		time.Sleep(wait)

		_, err := checker.CheckDomain(context.Background(), "a.test")
		require.NoError(t, err)
	}

	require.Equal(t, byName{"a.test.": 2}, r, "remembered within the TTL, asked again after it")
}

// gated blocks every MX lookup until open is closed, and counts them.
type gated struct {
	entered chan struct{}
	open    chan struct{}
	calls   atomic.Int32
}

func (g *gated) LookupMX(context.Context, string) ([]*net.MX, error) {
	if g.calls.Add(1) == 1 {
		close(g.entered)
	}

	<-g.open

	return []*net.MX{{Host: "mx.a.test.", Pref: 10}}, nil
}

func (g *gated) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return nil, nil }

// Concurrent checks of one domain share one lookup, and only the call that
// made it reports it. The cache is off, so only the merging can save them.
func TestCheckDomain_ConcurrentChecksShareALookup(t *testing.T) {
	t.Parallel()

	g := &gated{entered: make(chan struct{}), open: make(chan struct{})}
	checker := emailaddr.NewChecker(emailaddr.WithResolver(g), emailaddr.WithCacheTTL(0))

	const n = 8

	var (
		wg     sync.WaitGroup
		looked atomic.Int32
		errs   = make(chan error, n)
	)

	for range n {
		wg.Go(func() {
			l, err := checker.CheckDomain(context.Background(), "A.test")
			errs <- err

			if l {
				looked.Add(1)
			}
		})
	}

	<-g.entered
	// Let the other calls join the lookup in flight: they start within
	// microseconds, so the wait is a wide margin, not a guess.
	time.Sleep(200 * time.Millisecond)
	close(g.open)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	require.Equal(t, int32(1), g.calls.Load())
	require.Equal(t, int32(1), looked.Load())
}

// byName answers that every domain has an MX, and counts lookups by name.
type byName map[string]int

func (b byName) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	b[name]++
	return []*net.MX{{Host: "mx." + name, Pref: 10}}, nil
}

func (b byName) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, nil
}

func TestCheckDomain_CacheSize(t *testing.T) {
	t.Parallel()

	r := byName{}
	checker := emailaddr.NewChecker(emailaddr.WithResolver(r), emailaddr.WithCacheSize(2))
	ctx := context.Background()

	for _, domain := range []string{"a.test", "b.test", "a.test", "c.test", "a.test", "b.test"} {
		_, err := checker.CheckDomain(ctx, domain)
		require.NoError(t, err)
	}

	// c.test pushed out b.test, the least recently used; a.test stayed.
	require.Equal(t, byName{"a.test.": 1, "b.test.": 2, "c.test.": 1}, r)
}

func TestCheckDomain_CacheOff(t *testing.T) {
	t.Parallel()

	for name, opt := range map[string]emailaddr.Option{"ttl 0": emailaddr.WithCacheTTL(0), "size 0": emailaddr.WithCacheSize(0)} {
		r := byName{}
		checker := emailaddr.NewChecker(emailaddr.WithResolver(r), opt)

		for range 2 {
			_, err := checker.CheckDomain(context.Background(), "a.test")
			require.NoError(t, err)
		}

		require.Equal(t, byName{"a.test.": 2}, r, name)
	}
}

func TestChecker_AllowsDisposable(t *testing.T) {
	require.False(t, emailaddr.NewChecker().AllowsDisposable())
	require.True(t, emailaddr.NewChecker(emailaddr.WithAllowDisposable(true)).AllowsDisposable())
}
