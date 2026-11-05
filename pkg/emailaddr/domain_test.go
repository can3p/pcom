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
