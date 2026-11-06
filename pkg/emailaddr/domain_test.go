package emailaddr_test

import (
	"context"
	"errors"
	"net"
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

			err := checker.CheckDomain(ctx, "Mail.test")
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
	err := emailaddr.NewChecker(emailaddr.WithResolver(r), emailaddr.WithTimeout(0)).CheckDomain(context.Background(), "mail.test")

	require.NoError(t, err)
	require.Empty(t, r.lookup)
}

// clock is a settable time for one checker.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestCheckDomain_Cache(t *testing.T) {
	t.Parallel()

	notFound := &net.DNSError{Err: "no such host", Name: "mail.test.", IsNotFound: true}
	servfail := &net.DNSError{Err: "server misbehaving", Name: "mail.test.", IsTemporary: true}
	mx := []*net.MX{{Host: "mx.mail.test.", Pref: 10}}
	ttl := time.Minute

	tests := []struct {
		name     string
		resolver *fakeResolver
		want     error
		cached   bool
	}{
		{"accepted", &fakeResolver{mx: mx}, nil, true},
		{"refused", &fakeResolver{mxErr: notFound, ipErr: notFound}, emailaddr.ErrCantReceiveMail, true},
		{"SERVFAIL", &fakeResolver{mxErr: servfail}, nil, false},
		{"timeout", &fakeResolver{block: true}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &clock{t: time.Now()}
			checker := emailaddr.NewChecker(emailaddr.WithResolver(tt.resolver), emailaddr.WithTimeout(50*time.Millisecond),
				emailaddr.WithCacheTTL(ttl), emailaddr.WithClock(c.now))
			check := func() {
				t.Helper()

				err := checker.CheckDomain(context.Background(), "Mail.test")
				if tt.want == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, tt.want)
				}
			}

			check()
			first := len(tt.resolver.lookup)

			c.t = c.t.Add(ttl - time.Second)
			check()

			if !tt.cached {
				require.Len(t, tt.resolver.lookup, 2*first, "a failed lookup is asked again")
				return
			}

			require.Len(t, tt.resolver.lookup, first, "the answer is remembered within the TTL")

			c.t = c.t.Add(time.Second)
			check()
			require.Len(t, tt.resolver.lookup, 2*first, "the answer expires after the TTL")
		})
	}
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
		require.NoError(t, checker.CheckDomain(ctx, domain))
	}

	// c.test pushed out b.test, the least recently used; a.test stayed.
	require.Equal(t, byName{"a.test.": 1, "b.test.": 2, "c.test.": 1}, r)
}

func TestCheckDomain_CacheOff(t *testing.T) {
	t.Parallel()

	for name, opt := range map[string]emailaddr.Option{"ttl 0": emailaddr.WithCacheTTL(0), "size 0": emailaddr.WithCacheSize(0)} {
		r := byName{}
		checker := emailaddr.NewChecker(emailaddr.WithResolver(r), opt)

		require.NoError(t, checker.CheckDomain(context.Background(), "a.test"))
		require.NoError(t, checker.CheckDomain(context.Background(), "a.test"))
		require.Equal(t, byName{"a.test.": 2}, r, name)
	}
}

func TestChecker_AllowsDisposable(t *testing.T) {
	require.False(t, emailaddr.NewChecker().AllowsDisposable())
	require.True(t, emailaddr.NewChecker(emailaddr.WithAllowDisposable(true)).AllowsDisposable())
}
