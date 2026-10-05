package date

import (
	"fmt"
	"html/template"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/dustin/go-humanize"
)

const TimestampFormat = "Mon, 02 Jan 2006 15:04"

func LocalizeTime(user *model.User, t time.Time) time.Time {
	if user == nil {
		return t
	}

	l, err := time.LoadLocation(user.Timezone)
	if err != nil {
		return t
	}

	return t.In(l)
}

func FormatTimestamp(t time.Time, user *model.User) string {
	if user != nil {
		t = LocalizeTime(user, t)
	}
	return t.Format(TimestampFormat)
}

func RelativeTime(t time.Time, now time.Time) string {
	return humanize.RelTime(t, now, "ago", "from now")
}

func RenderTimeHTML(t time.Time, user *model.User, now time.Time) template.HTML {
	timestamp := FormatTimestamp(t, user)
	relative := RelativeTime(t, now)
	return template.HTML(fmt.Sprintf(`<span title="%s">%s</span>`, template.HTMLEscapeString(timestamp), template.HTMLEscapeString(relative)))
}
