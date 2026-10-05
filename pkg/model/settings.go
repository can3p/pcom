package model

import (
	"github.com/uptrace/bun"
)

// SystemSetting is a row of system_settings.
type SystemSetting struct {
	bun.BaseModel `bun:"table:system_settings"`

	ID               string `bun:"id,pk"`
	RegistrationOpen bool   `bun:"registration_open"`
}
