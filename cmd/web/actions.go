package main

import (
	"github.com/gin-gonic/gin"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func setupActions(r *gin.RouterGroup, db boil.ContextExecutor) {
}

//func reportError(c *gin.Context, s string) {
//c.JSON(http.StatusBadRequest, gin.H{
//"explanation": s,
//})
//}

//func reportSuccess(c *gin.Context) {
//c.JSON(http.StatusOK, gin.H{})
//}
