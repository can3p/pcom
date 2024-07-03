package util

import (
	"log"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
)

func LocalizeTime(user *core.User, t time.Time) time.Time {
	l, err := time.LoadLocation(user.Timezone)

	if err != nil {
		log.Printf("failed to parse timezone setting: [%s] - %v", user.Timezone, err)
		return t
	}

	return t.In(l)
}
