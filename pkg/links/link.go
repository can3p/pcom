package links

import (
	"os"

	"github.com/can3p/gogo/links"
	"github.com/can3p/pcom/pkg/util"
)

func DefaultAuthorizedHome() string {
	return Link("feed")
}

func Link(name string, args ...string) string {
	builder := links.NewArgBuilder(args...)

	var out string
	var fragment string

	switch name {
	case "default_authorized_home":
		out = DefaultAuthorizedHome()
	case "controls":
		out = "/controls"
	case "settings":
		out = "/controls/settings"
	case "write":
		out = "/write"
	case "feed":
		out = "/feed"
	case "privacy_policy":
		out = "/articles/privacy_policy"
	case "terms_of_service":
		out = "/articles/terms_of_service"
	case "post":
		out = "/posts/" + builder.Shift()
	case "shared_post":
		out = "/shared/" + builder.Shift()
	case "comment":
		postID := builder.Shift()
		commentID := builder.Shift()
		fragment = "comment" + postID + commentID

		out = "/posts/" + postID
	case "edit_post":
		out = "/posts/" + builder.Shift() + "/edit"
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
	case "form_edit_post":
		out = "/controls/form/edit_post"
	case "form_new_comment":
		out = "/controls/form/new_comment"
	case "form_save_settings":
		out = "/controls/form/save_settings"
	case "form_send_invite":
		out = "/controls/form/send_invite"
	case "form_change_password":
		out = "/controls/form/change_password"
	case "form_whitelist_connection":
		out = "/controls/form/whitelist_connection"
	case "form_prompt_post":
		out = "/controls/form/prompt_post"
	case "action":
		out = "/controls/action/" + builder.Shift()
	case "uploaded_media":
		out = "/user-media/" + builder.Shift()
	case "login":
		out = "/login"
	case "signup":
		out = "/signup"
	}

	l := out + builder.BuildQueryString()

	if fragment != "" {
		l = l + "#" + fragment
	}

	return l
}

func AbsLink(name string, args ...string) string {
	if name == "uploaded_media" {
		if pr, ok := os.LookupEnv("USER_MEDIA_CDN"); ok && util.InCluster() {
			return pr + "/" + args[0]
		}
	}

	return util.SiteRoot() + Link(name, args...)
}
