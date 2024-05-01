package pgsession

import (
	"crypto/sha256"
	"fmt"
)

func HashUserPwd(email, password string) string {
	data := []byte(email + ":" + password)
	hash := sha256.Sum256(data)

	return fmt.Sprintf("%x", hash)
}
