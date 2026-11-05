// Package abuse keeps automated submissions out of the public forms that
// mail an address anyone types in: a proof-of-work challenge the browser
// solves, a honeypot field, and a minimum time between showing a form and
// submitting it. It knows nothing of the forms that use it: a caller issues
// a challenge, renders it, and passes the submitted fields to Verify.
package abuse

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

// DefaultPowDifficulty is how many leading zero bits a proof-of-work answer's
// hash needs when the configuration sets none; it is also the configuration's
// default.
const DefaultPowDifficulty = 15

// DefaultPowMaxAge is how long a proof-of-work challenge may be answered
// when the configuration sets none; it is also the configuration's default.
const DefaultPowMaxAge = 30 * time.Minute

// DefaultMinFillTime is the least time between showing a form and submitting
// it when the configuration sets none; it is also the configuration's default.
const DefaultMinFillTime = 2 * time.Second

// Names of the form fields a protected form carries. The proof-of-work
// controller fills PowNonceField; HoneypotField is an input a person never
// sees (hidden with CSS), so a script that fills every input fills it.
const (
	PowChallengeField = "pow_challenge"
	PowNonceField     = "pow_nonce"
	HoneypotField     = "website"
)

var (
	// ErrInvalid is a submission to refuse with a message: the challenge is
	// forged, expired or already used, or the nonce does not solve it. The
	// reader gets a new challenge by reloading the form.
	ErrInvalid = errors.New("abuse: invalid proof of work")
	// ErrSilent is a submission a script made (the honeypot is filled, or
	// the form came back faster than a person fills it). The caller answers
	// as if it succeeded and does nothing, so a script learns nothing.
	ErrSilent = errors.New("abuse: automated submission")
)

// Challenge is what the server hands a form. Token is its signed, encoded
// form, `<issued unix seconds>.<salt>.<difficulty>.<signature>`, and is what
// the browser hashes: SHA-256 over the UTF-8 bytes of Token followed by the
// decimal digits of the nonce, which must start with Difficulty zero bits.
type Challenge struct {
	IssuedAt   time.Time
	Salt       string
	Difficulty int
	Token      string
}

// Submission is the abuse-related part of a posted form.
type Submission struct {
	Token    string // PowChallengeField
	Nonce    string // PowNonceField
	Honeypot string // HoneypotField
}

// UsedStore records which challenges were answered; *repo.Store is one.
type UsedStore interface {
	// UsePowChallenge records salt as used and reports whether it was new.
	UsePowChallenge(ctx context.Context, salt string, at time.Time) (bool, error)
	// DeletePowChallengesBefore forgets the challenges used before t.
	DeletePowChallengesBefore(ctx context.Context, t time.Time) error
}

// Guard issues and verifies challenges.
type Guard struct {
	key        []byte
	difficulty int
	maxAge     time.Duration
	minFill    time.Duration
	now        func() time.Time
}

// Option configures a Guard.
type Option func(*Guard)

// WithDifficulty sets the leading zero bits required; zero or less keeps
// DefaultPowDifficulty.
func WithDifficulty(n int) Option {
	return func(g *Guard) {
		if n > 0 {
			g.difficulty = n
		}
	}
}

// WithMaxAge sets how long a challenge may be answered; zero or less keeps
// DefaultPowMaxAge.
func WithMaxAge(d time.Duration) Option {
	return func(g *Guard) {
		if d > 0 {
			g.maxAge = d
		}
	}
}

// WithMinFillTime sets the least time between issue and submit; zero or less
// keeps DefaultMinFillTime.
func WithMinFillTime(d time.Duration) Option {
	return func(g *Guard) {
		if d > 0 {
			g.minFill = d
		}
	}
}

// WithClock replaces the clock, for tests.
func WithClock(now func() time.Time) Option {
	return func(g *Guard) { g.now = now }
}

// New builds a Guard that signs challenges with a key derived from secret
// (the session salt).
func New(secret string, opts ...Option) *Guard {
	sum := sha256.Sum256([]byte("pcom/abuse/pow:" + secret))
	g := &Guard{
		key:        sum[:],
		difficulty: DefaultPowDifficulty,
		maxAge:     DefaultPowMaxAge,
		minFill:    DefaultMinFillTime,
		now:        time.Now,
	}

	for _, o := range opts {
		o(g)
	}

	return g
}

// Difficulty is the leading zero bits challenges ask for.
func (g *Guard) Difficulty() int { return g.difficulty }

// MaxAge is how long a challenge may be answered.
func (g *Guard) MaxAge() time.Duration { return g.maxAge }

// MinFillTime is the least time between issue and submit.
func (g *Guard) MinFillTime() time.Duration { return g.minFill }

func (g *Guard) sign(payload string) string {
	m := hmac.New(sha256.New, g.key)
	m.Write([]byte(payload))

	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Issue makes a fresh challenge.
func (g *Guard) Issue() Challenge {
	var raw [16]byte

	_, _ = rand.Read(raw[:]) // never fails

	c := Challenge{
		IssuedAt:   g.now().Truncate(time.Second),
		Salt:       hex.EncodeToString(raw[:]),
		Difficulty: g.difficulty,
	}
	payload := fmt.Sprintf("%d.%s.%d", c.IssuedAt.Unix(), c.Salt, c.Difficulty)
	c.Token = payload + "." + g.sign(payload)

	return c
}

// parse reads a token and checks its signature.
func (g *Guard) parse(token string) (Challenge, error) {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return Challenge{}, ErrInvalid
	}

	payload, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(g.sign(payload))) {
		return Challenge{}, ErrInvalid
	}

	parts := strings.Split(payload, ".")
	if len(parts) != 3 {
		return Challenge{}, ErrInvalid
	}

	unix, err1 := strconv.ParseInt(parts[0], 10, 64)
	n, err2 := strconv.Atoi(parts[2])

	if err1 != nil || err2 != nil {
		return Challenge{}, ErrInvalid
	}

	return Challenge{IssuedAt: time.Unix(unix, 0), Salt: parts[1], Difficulty: n, Token: token}, nil
}

// leadingZeroBits counts the zero bits a hash starts with.
func leadingZeroBits(sum []byte) int {
	n := 0

	for _, b := range sum {
		if b == 0 {
			n += 8
			continue
		}

		return n + bits.LeadingZeros8(b)
	}

	return n
}

// Verify checks a submission and, if it is good, records its challenge as
// used on store (pass a transaction so the use and the caller's write commit
// together). It returns ErrSilent for the honeypot and the fill time,
// ErrInvalid for anything else wrong, and nil when the submission may
// proceed. A refused submission does not use its challenge, except a replay,
// which is refused because the challenge was used.
func (g *Guard) Verify(ctx context.Context, store UsedStore, sub Submission) error {
	if sub.Honeypot != "" {
		return ErrSilent
	}

	c, err := g.parse(sub.Token)
	if err != nil {
		return err
	}

	now := g.now()

	age := now.Sub(c.IssuedAt)
	if age > g.maxAge || c.Difficulty < g.difficulty {
		return ErrInvalid
	}

	if age < g.minFill {
		return ErrSilent
	}

	sum := sha256.Sum256([]byte(sub.Token + sub.Nonce))
	if leadingZeroBits(sum[:]) < c.Difficulty {
		return ErrInvalid
	}

	fresh, err := store.UsePowChallenge(ctx, c.Salt, now)
	if err != nil {
		return err
	}

	if !fresh {
		return ErrInvalid
	}

	return nil
}

// Prune forgets the used challenges too old to be answered anyway, so the
// table stays small. Something has to call it now and then.
func (g *Guard) Prune(ctx context.Context, store UsedStore) error {
	return store.DeletePowChallengesBefore(ctx, g.now().Add(-g.maxAge))
}
