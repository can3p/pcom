package config

import "github.com/can3p/gogo/settings"

// Seed is the settings of the seed command, which fills a development
// database.
type Seed struct {
	Database Database `group:"Database"`
	SiteRoot string   `long:"site-root" env:"SITE_ROOT" description:"Public root URL of the site, used in seeded links"`
	Reset    bool     `long:"reset" description:"Truncate every table except migrations and system_settings first"`

	// Production is the guard: fly.io sets FLY_APP_NAME, and seed refuses to run there.
	Production string `long:"production-guard" env:"FLY_APP_NAME" hidden:"true"`
}

// AdminInvite is the settings of `admin invite`.
type AdminInvite struct {
	Database Database `group:"Database"`
	Email    string   `long:"email" description:"Email of the account that receives the invites" required:"true"`
	Num      int      `long:"num" description:"Number of invites to add" required:"true"`
}

// AdminRegistration is the settings of `admin registration`.
type AdminRegistration struct {
	Database Database `group:"Database"`
	Open     bool     `long:"open" description:"Open registration"`
	Close    bool     `long:"close" description:"Close registration"`
}

// AdminLoginCode is the settings of `admin login-code`.
type AdminLoginCode struct {
	Database    Database        `group:"Database"`
	Email       string          `long:"email" description:"Email of the account to issue a login code for" required:"true"`
	SessionSalt settings.Secret `long:"session-salt" env:"SESSION_SALT" description:"Salt for session cookies and hashed values" required:"true"`
	// Login must match the server's, so the code is judged by the same limits.
	Login Login `group:"Login"`
}
