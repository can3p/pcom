// Package abuse keeps automated submissions out of the public forms that
// mail an address anyone types in: a proof-of-work challenge the browser
// solves, and a minimum time between showing a form and submitting it.
package abuse

import "time"

// DefaultPowDifficulty is how many leading zero bits a proof-of-work answer's
// hash needs when the configuration sets none; it is also the configuration's
// default.
const DefaultPowDifficulty = 18

// DefaultPowMaxAge is how long a proof-of-work challenge may be answered
// when the configuration sets none; it is also the configuration's default.
const DefaultPowMaxAge = 30 * time.Minute

// DefaultMinFillTime is the least time between showing a form and submitting
// it when the configuration sets none; it is also the configuration's default.
const DefaultMinFillTime = 2 * time.Second
