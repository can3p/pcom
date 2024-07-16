package csrf

import (
	"net/http"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/gin-gonic/gin"
)

func CheckCSRF(c *gin.Context) {
	data := auth.GetUserData(c)
	csrfToken := c.GetHeader("X-CSRFToken")

	if csrfToken == "" {
		if val, ok := c.GetPostForm("header_csrf"); ok {
			csrfToken = val
		}
	}

	if csrfToken == "" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	if data.CSRFToken == "" {
		// abnormal situation, should never happen
		panic("session does not contain csrf token")
	}

	if csrfToken != data.CSRFToken {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	c.Next()
}
