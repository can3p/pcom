// Package registry builds every service over one store. It is part of the
// composition root: transports get their services from here and never see
// pkg/repo.
package registry

import (
	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/jmoiron/sqlx"
)

// Deps is what services are built from besides the database.
type Deps struct {
	Sender       sender.Sender
	MediaStorage server.MediaStorage
}

// Services is one field per area service.
type Services struct {
	Shares *shares.Service
}

func New(db *sqlx.DB, _ Deps) *Services {
	store := repo.New(db)

	return &Services{
		Shares: shares.New(store),
	}
}
