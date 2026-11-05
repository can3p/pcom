package abuse_test

import (
	"context"
	"crypto/sha256"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// testBits keeps solving cheap; the real difficulty is the browser test's.
const testBits = 8

// solve finds a nonce for the challenge the way the browser does.
func solve(t *testing.T, token string, bits int) string {
	t.Helper()

	for n := range 1 << 24 {
		nonce := strconv.Itoa(n)
		sum := sha256.Sum256([]byte(token + nonce))

		zeros := 0

		for _, b := range sum {
			if b != 0 {
				for mask := byte(0x80); mask != 0 && b&mask == 0; mask >>= 1 {
					zeros++
				}

				break
			}

			zeros += 8
		}

		if zeros >= bits {
			return nonce
		}
	}

	t.Fatal("no nonce found")

	return ""
}

func TestVerify(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)
	now := time.Now()

	// The guard's clock is the test's: each row sets how long after issue
	// the form comes back.
	var elapsed time.Duration

	clock := func() time.Time { return now.Add(elapsed) }
	g := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(clock))
	other := abuse.New("another secret", abuse.WithDifficulty(testBits), abuse.WithClock(clock))

	for _, r := range []struct {
		name    string
		elapsed time.Duration
		mutate  func(c abuse.Challenge, s *abuse.Submission)
		want    error
	}{
		{"solved", abuse.DefaultMinFillTime, nil, nil},
		{"forged signature", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) {
			s.Token = other.Issue().Token
			s.Nonce = solve(t, s.Token, testBits)
		}, abuse.ErrInvalid},
		{"tampered difficulty", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) {
			parts := strings.Split(c.Token, ".")
			parts[2] = "1"
			s.Token = strings.Join(parts, ".")
			s.Nonce = solve(t, s.Token, 1)
		}, abuse.ErrInvalid},
		{"garbage token", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) { s.Token = "x" }, abuse.ErrInvalid},
		{"expired", abuse.DefaultPowMaxAge + time.Minute, nil, abuse.ErrInvalid},
		{"wrong nonce", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) { s.Nonce = "not-it" }, abuse.ErrInvalid},
		{"honeypot filled", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) { s.Honeypot = "http://spam" }, abuse.ErrSilent},
		{"too fast", time.Second, nil, abuse.ErrSilent},
	} {
		t.Run(r.name, func(t *testing.T) {
			elapsed = 0
			c := g.Issue()
			sub := abuse.Submission{Token: c.Token, Nonce: solve(t, c.Token, testBits)}

			if r.mutate != nil {
				r.mutate(c, &sub)
			}

			elapsed = r.elapsed

			require.ErrorIs(t, g.Verify(ctx, store, sub), r.want)

			// Only an accepted submission uses its challenge.
			if r.want != nil && r.name != "forged signature" && r.name != "garbage token" {
				used, err := store.UsePowChallenge(ctx, c.Salt, now)
				require.NoError(t, err)
				require.True(t, used, "a refused submission used the challenge")
			}
		})
	}
}

func TestVerify_ReplayAndPrune(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)
	now := time.Now()
	elapsed := time.Duration(0)
	g := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(func() time.Time { return now.Add(elapsed) }))

	c := g.Issue()
	sub := abuse.Submission{Token: c.Token, Nonce: solve(t, c.Token, testBits)}
	elapsed = abuse.DefaultMinFillTime

	require.NoError(t, g.Verify(ctx, store, sub))
	require.ErrorIs(t, g.Verify(ctx, store, sub), abuse.ErrInvalid, "the same answer twice")

	// Once the challenge is too old to answer, forgetting its salt is safe.
	elapsed = abuse.DefaultPowMaxAge + time.Minute
	require.NoError(t, g.Prune(ctx, store))

	used, err := store.UsePowChallenge(ctx, c.Salt, now)
	require.NoError(t, err)
	require.True(t, used, "pruned")
}

func TestGuard_Options(t *testing.T) {
	t.Parallel()

	g := abuse.New("s", abuse.WithDifficulty(5), abuse.WithMaxAge(time.Hour), abuse.WithMinFillTime(time.Minute))
	require.Equal(t, 5, g.Difficulty())
	require.Equal(t, time.Hour, g.MaxAge())
	require.Equal(t, time.Minute, g.MinFillTime())

	d := abuse.New("s", abuse.WithDifficulty(0), abuse.WithMaxAge(0), abuse.WithMinFillTime(0))
	require.Equal(t, abuse.DefaultPowDifficulty, d.Difficulty())
	require.Equal(t, abuse.DefaultPowMaxAge, d.MaxAge())
	require.Equal(t, abuse.DefaultMinFillTime, d.MinFillTime())
}
