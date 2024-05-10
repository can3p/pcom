package main

import (
	"fmt"
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func setupActions(r *gin.RouterGroup, db *sqlx.DB) {
	r.POST("/remove_from_whitelist", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			UserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DropConnectionGrant(c, db, dbUser.ID, input.UserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/create_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.EstablishConnection(c, db, dbUser.ID, input.TargetUserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

	r.POST("/drop_connection", func(c *gin.Context) {
		userData := auth.GetUserData(c)
		dbUser := userData.DBUser

		var input struct {
			TargetUserID string `json:"userId"`
		}

		if err := c.BindJSON(&input); err != nil {
			reportError(c, fmt.Sprintf("Bad input: %s", err.Error()))
			return
		}

		if err := userops.DropConnection(c, db, dbUser.ID, input.TargetUserID); err != nil {
			reportError(c, fmt.Sprintf("Failed operation: %s", err.Error()))
			return
		}

		reportSuccess(c)
	})

}

func reportError(c *gin.Context, s string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"explanation": s,
	})
}

func reportSuccess(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{})
}
