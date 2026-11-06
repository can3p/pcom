package abuse_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"math/bits"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

// testBits keeps solving cheap; the real difficulty is the browser test's.
const testBits = 8

// zeros counts the leading zero bits of the hash of token and nonce.
func zeros(token, nonce string) int {
	sum := sha256.Sum256([]byte(token + nonce))
	n := 0

	for _, b := range sum {
		if b != 0 {
			return n + bits.LeadingZeros8(b)
		}

		n += 8
	}

	return n
}

// nonceFor finds a nonce that solves token at the given bits, or, with
// solves false, one that verifiably does not.
func nonceFor(t *testing.T, token string, diff int, solves bool) string {
	t.Helper()

	for n := range 1 << 24 {
		if nonce := strconv.Itoa(n); (zeros(token, nonce) >= diff) == solves {
			return nonce
		}
	}

	t.Fatal("no nonce found")

	return ""
}

func solve(t *testing.T, token string, diff int) string { return nonceFor(t, token, diff, true) }

// failStore is a UsedStore whose every call fails with err.
type failStore struct{ err error }

func (s failStore) UsePowChallenge(context.Context, string, time.Time) (bool, error) {
	return false, s.err
}

func (s failStore) DeletePowChallengesBefore(context.Context, time.Time) error { return s.err }

// whole seconds, so a challenge's truncated issue time is exactly now.
func start() time.Time { return time.Now().Truncate(time.Second) }

func TestVerify(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)
	now := start()

	// The guard's clock is the test's: each row sets how long after issue
	// the form comes back.
	var elapsed time.Duration

	clock := func() time.Time { return now.Add(elapsed) }
	g := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(clock))
	other := abuse.New("another secret", abuse.WithDifficulty(testBits), abuse.WithClock(clock))
	low := abuse.New("secret", abuse.WithDifficulty(4), abuse.WithClock(clock))

	for _, r := range []struct {
		name    string
		elapsed time.Duration
		mutate  func(c abuse.Challenge, s *abuse.Submission)
		want    error
		ownSalt bool // the submission carries the row's own challenge, so its salt is checked afterwards
	}{
		{"solved", abuse.DefaultMinFillTime, nil, nil, true},
		{"exactly max age", abuse.DefaultPowMaxAge, nil, nil, true},
		{"past max age", abuse.DefaultPowMaxAge + time.Second, nil, abuse.ErrInvalid, true},
		{"expired", abuse.DefaultPowMaxAge + time.Minute, nil, abuse.ErrInvalid, true},
		{"too fast", abuse.DefaultMinFillTime - time.Second, nil, abuse.ErrSilent, true},
		{"forged signature", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) {
			s.Token = other.Issue().Token
			s.Nonce = solve(t, s.Token, testBits)
		}, abuse.ErrInvalid, false},
		{"signed below the floor", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) {
			s.Token = low.Issue().Token
			s.Nonce = solve(t, s.Token, 4)
		}, abuse.ErrInvalid, false},
		{"garbage token", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) { s.Token = "x" }, abuse.ErrInvalid, false},
		{"wrong nonce", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) {
			s.Nonce = nonceFor(t, c.Token, testBits, false)
		}, abuse.ErrInvalid, true},
		{"no nonce", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) { s.Nonce = "" }, abuse.ErrInvalid, true},
		{"honeypot filled", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) { s.Honeypot = "http://spam" }, abuse.ErrSilent, true},
		{"honeypot and bad token", abuse.DefaultMinFillTime, func(c abuse.Challenge, s *abuse.Submission) {
			s.Token, s.Honeypot = "x", "http://spam"
		}, abuse.ErrSilent, false},
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
			if r.ownSalt {
				fresh, err := store.UsePowChallenge(ctx, c.Salt, now)
				require.NoError(t, err)
				require.Equal(t, r.want != nil, fresh, "salt recorded exactly when accepted")
			}
		})
	}
}

func TestVerify_ReplayAndPrune(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)
	now := start()
	elapsed := time.Duration(0)
	g := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(func() time.Time { return now.Add(elapsed) }))

	c := g.Issue()
	sub := abuse.Submission{Token: c.Token, Nonce: solve(t, c.Token, testBits)}
	elapsed = abuse.DefaultMinFillTime

	require.NoError(t, g.Verify(ctx, store, sub))
	require.ErrorIs(t, g.Verify(ctx, store, sub), abuse.ErrInvalid, "the same answer twice")

	// A challenge issued and answered much later is recent when the first is pruned.
	elapsed = abuse.DefaultPowMaxAge + 30*time.Second
	recent := g.Issue()
	elapsed += abuse.DefaultMinFillTime

	require.NoError(t, g.Verify(ctx, store, abuse.Submission{Token: recent.Token, Nonce: solve(t, recent.Token, testBits)}))

	// Once the first challenge is too old to answer, forgetting its salt is safe.
	elapsed = abuse.DefaultPowMaxAge + time.Minute
	require.NoError(t, g.Prune(ctx, store))

	used, err := store.UsePowChallenge(ctx, c.Salt, now)
	require.NoError(t, err)
	require.True(t, used, "pruned")

	used, err = store.UsePowChallenge(ctx, recent.Salt, now)
	require.NoError(t, err)
	require.False(t, used, "a recent salt survives the prune")
}

func TestVerify_ConcurrentReplay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repo.New(testdb.New(t).DB)
	now := start()
	g := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(func() time.Time { return now.Add(abuse.DefaultMinFillTime) }))
	// Issued at the guard's clock, which is already minFill later.
	c := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(func() time.Time { return now })).Issue()
	sub := abuse.Submission{Token: c.Token, Nonce: solve(t, c.Token, testBits)}

	errs := make([]error, 2)

	var wg sync.WaitGroup

	for i := range errs {
		wg.Go(func() { errs[i] = g.Verify(ctx, store, sub) })
	}

	wg.Wait()

	require.NotEqual(t, errs[0] == nil, errs[1] == nil, "exactly one verification succeeds: %v", errs)
	require.ErrorIs(t, errors.Join(errs...), abuse.ErrInvalid)
}

func TestVerify_StoreError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	now := start()
	g := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(func() time.Time { return now.Add(abuse.DefaultMinFillTime) }))
	c := abuse.New("secret", abuse.WithDifficulty(testBits), abuse.WithClock(func() time.Time { return now })).Issue()

	err := g.Verify(context.Background(), failStore{boom}, abuse.Submission{Token: c.Token, Nonce: solve(t, c.Token, testBits)})
	require.ErrorIs(t, err, boom)
	require.NotErrorIs(t, err, abuse.ErrInvalid)
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
