package app

import (
	"database/sql"
	"fmt"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"

	"github.com/can3p/gogo/util/transact"
)

// mountSettingsActions registers the account settings actions.
func mountSettingsActions(d *Deps, r *gin.RouterGroup) {
	db := d.DB

	r.POST("/generate_api_key", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		id, err := uuid.NewV7()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		newApiKey, err := uuid.NewV7()

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		err = transact.Transact(db, func(tx *sql.Tx) error {
			record := core.UserAPIKey{
				ID:     id.String(),
				APIKey: newApiKey.String(),
				UserID: dbUser.ID,
			}

			// no key rotation for now
			// feel free to implement/change
			return record.Upsert(c, tx, false, []string{core.UserAPIKeyColumns.UserID}, boil.Infer(), boil.Infer())
		})

		if err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	// creates the private feed token or replaces it; the old feed URL stops working
	r.POST("/regenerate_feed_token", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		if _, err := repo.RegenerateFeedToken(c.Request.Context(), db, userData.DBUser.ID); err != nil {
			reportError(c, fmt.Sprintf("Operation Failed: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})
}
