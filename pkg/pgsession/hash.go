package pgsession

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"golang.org/x/crypto/argon2"
)

// argon2id parameters, the OWASP minimum: 19 MiB of memory, two passes, one
// thread. They are written into every hash, so raising them later only
// affects new hashes.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
	argonPrefix  = "$argon2id$"
)

// NormalizeEmail is the form an email address is stored and looked up in.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// HashUserPwd is the legacy password hash, sha256(email + ":" + password)
// with no salt or work factor. New hashes come from HashPassword; this one
// is only kept to verify passwords stored before the switch.
func HashUserPwd(email, password string) string {
	data := []byte(email + ":" + password)
	hash := sha256.Sum256(data)

	return fmt.Sprintf("%x", hash)
}

// HashPassword returns an argon2id hash of password with a random salt, in
// the PHC string format ($argon2id$v=19$m=...,t=...,p=...$salt$key).
func HashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	_, _ = rand.Read(salt) // never fails, see crypto/rand.Read

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding

	return fmt.Sprintf("%sv=%d$m=%d,t=%d,p=%d$%s$%s",
		argonPrefix, argon2.Version, argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key))
}

// CheckUserPwd reports whether password matches the stored hash of a user
// whose stored email is email. A legacy hash was computed from the email as
// it was stored when the password was set, so the caller passes the stored
// spelling, not what the user typed. needsRehash is true when a legacy hash
// matched, so the caller can replace it with HashPassword.
func CheckUserPwd(stored, email, password string) (ok bool, needsRehash bool) {
	if strings.HasPrefix(stored, argonPrefix) {
		return checkArgon2id(stored, password), false
	}

	if subtle.ConstantTimeCompare([]byte(HashUserPwd(email, password)), []byte(stored)) == 1 {
		return true, true
	}

	return false, false
}

// EmailIs matches the users whose email equals email ignoring case and
// surrounding space, the way emails are compared (backed by an index on
// lower(btrim(email))). Legacy accounts may store mixed case, and on legacy
// collisions it matches more than one user.
func EmailIs(email string) qm.QueryMod {
	return qm.Where("lower(btrim(email)) = ?", NormalizeEmail(email))
}

func checkArgon2id(stored, password string) bool {
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	parts := strings.Split(stored, "$")
	if len(parts) != 6 {
		return false
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil || time == 0 || threads == 0 {
		return false
	}

	enc := base64.RawStdEncoding

	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false
	}

	key, err := enc.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return false
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(key)))

	return subtle.ConstantTimeCompare(got, key) == 1
}
