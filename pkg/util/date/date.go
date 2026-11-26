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
	return template.HTML(fmt.Sprintf(`<span title="%s">%s</span>`, template.HTMLEscapeString(timestamp), template.HTMLEscapeString(relative))) //nolint:gosec // both values are escaped
}

// RecentWindow is how long a date renders as relative text ("3 hours ago")
// before it switches to the calendar date.
const RecentWindow = 24 * time.Hour

const (
	dateFormat         = "2 Jan"
	dateFormatWithYear = "2 Jan 2006"
	tooltipZoneFormat  = "Mon, 02 Jan 2006 15:04 MST"
)

// RenderDateHTML renders t as a <time> element. Within RecentWindow the text is
// relative, otherwise it is the calendar date in the user's timezone (without
// the year when it is the current one). The tooltip always carries the full
// timestamp with its timezone and the relative time.
func RenderDateHTML(t time.Time, user *model.User, now time.Time) template.HTML {
	local := LocalizeTime(user, t)
	relative := RelativeTime(t, now)

	var text string

	switch {
	case t.After(now.Add(-RecentWindow)) && !t.After(now.Add(RecentWindow)):
		text = relative
	case local.Year() == LocalizeTime(user, now).Year():
		text = local.Format(dateFormat)
	default:
		text = local.Format(dateFormatWithYear)
	}

	title := local.Format(tooltipZoneFormat) + " · " + relative

	return template.HTML(fmt.Sprintf(`<time datetime="%s" title="%s">%s</time>`, //nolint:gosec // all values are escaped
		template.HTMLEscapeString(t.UTC().Format(time.RFC3339)),
		template.HTMLEscapeString(title),
		template.HTMLEscapeString(text)))
}
