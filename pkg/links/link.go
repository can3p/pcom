package links

import (
	"github.com/can3p/gogo/links"
	"github.com/can3p/pcom/pkg/util"
)

func Link(name string, args ...string) string {
	builder := links.NewArgBuilder(args...)

	var out string

	switch name {
	case "controls":
		out = "/controls"
	case "settings":
		out = "/controls/settings"
	case "write":
		out = "/controls/write"
	case "direct_feed":
		out = "/controls/feed/direct"
	case "explore_feed":
		out = "/controls/feed/explore"
	case "privacy_policy":
		out = "/articles/privacy_policy"
	case "terms_of_service":
		out = "/articles/terms_of_service"
	case "post":
		out = "/posts/" + builder.Shift()
	case "user":
		out = "/users/" + builder.Shift()
	case "article":
		out = "/articles/" + builder.Shift()
	case "invite":
		out = "/invite/" + builder.Shift()
	case "use_case":
		out = "/use-case/" + builder.Shift()
	case "form_signup_waiting_list":
		out = "/form/signup_waiting_list"
	case "form_signup":
		out = "/form/signup"
	case "form_accept_invite":
		out = "/form/accept_invite/" + builder.Shift()
	case "form_login":
		out = "/form/login"
	case "confirm_waiting_list":
		out = "/confirm_waiting_list/" + builder.Shift()
	case "confirm_signup":
		out = "/confirm_signup/" + builder.Shift()
	case "form_new_post":
		out = "/controls/form/new_post"
	case "form_save_settings":
		out = "/controls/form/save_settings"
	case "form_send_invite":
		out = "/controls/form/send_invite"
	case "form_change_password":
		out = "/controls/form/change_password"
	case "form_whitelist_connection":
		out = "/controls/form/whitelist_connection"
	case "action":
		out = "/controls/action/" + builder.Shift()
	case "uploaded_media":
		out = "/user-media/" + builder.Shift()
	case "logout":
		out = "/logout"
	case "login":
		out = "/login"
	case "signup":
		out = "/signup"
	}

	return out + builder.BuildQueryString()
}

func AbsLink(name string, args ...string) string {
	return util.SiteRoot() + Link(name, args...)
}
