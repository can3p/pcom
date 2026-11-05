package app

import (
	"errors"
	"net/http"

	gogoForms "github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/logging"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
)

// unsubscribedNotice is what the manage page says after an unsubscribe.
const unsubscribedNotice = "You are unsubscribed."

// subscribeOffer is the subscribe form a page of author's shows, or nil when
// the visitor has an account (they follow through their connections) or the
// author takes no subscriptions.
func subscribeOffer(d *Deps, userData auth.UserData, author *model.User) *web.SubscribeOffer {
	if userData.IsLoggedIn || !d.Services.Subscriptions.Subscribable(author) {
		return nil
	}

	return &web.SubscribeOffer{Username: author.Username, Challenge: d.Guard.Issue().Token}
}

// tokenPage marks the response as one a link's token opens: not to be cached,
// indexed or leaked to the next site through the referrer.
func tokenPage(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Robots-Tag", "noindex")
	c.Header("Cache-Control", "no-store")
}

// orExpired turns the service's answer for a token it does not know (or no
// longer honors) into a nil result, which the pages show as an expired link.
func orExpired[T any](v *T, err error) (*T, error) {
	if errors.Is(err, service.ErrNotFound) {
		return nil, nil
	}

	return v, err
}

// mountSubscriptionRoutes registers email subscriptions: the public forms, the
// pages the mailed links open (a link's token authorizes them, so they take
// no CSRF token: a request without the token does nothing) and the author's
// removal of a subscriber.
func mountSubscriptionRoutes(d *Deps, r, actions, nonControlsForms *gin.RouterGroup) {
	subs := d.Services.Subscriptions

	nonControlsForms.POST("/subscribe/:username", func(c *gin.Context) {
		gogoForms.DefaultHandler(c, logging.FromContext(c), forms.SubscribeFormNew(subs, d.Guard, c.Param("username")))
	})

	nonControlsForms.POST("/subscriptions/lost", func(c *gin.Context) {
		gogoForms.DefaultHandler(c, logging.FromContext(c), forms.SubscriptionsLostFormNew(subs, d.Guard))
	})

	r.GET("/subscriptions", func(c *gin.Context) {
		userData := auth.GetUserData(c)

		c.HTML(http.StatusOK, "subscriptions_lost.html", web.SubscriptionsLost(c, &userData, d.Guard.Issue().Token))
	})

	r.GET("/subscriptions/confirm/:token", func(c *gin.Context) {
		tokenPage(c)

		userData := auth.GetUserData(c)

		view, err := orExpired(subs.ConfirmView(c, c.Param("token")))
		if err != nil {
			pageError(c, err)
			return
		}

		c.HTML(http.StatusOK, "subscription_confirm.html", web.SubscriptionConfirm(c, &userData, c.Param("token"), view, false))
	})

	r.POST("/subscriptions/confirm/:token", func(c *gin.Context) {
		tokenPage(c)

		userData := auth.GetUserData(c)
		token := c.Param("token")

		view, err := orExpired(subs.ConfirmView(c, token))
		if err == nil && view != nil {
			err = subs.Confirm(c, token)
		}

		if errors.Is(err, service.ErrNotFound) {
			view, err = nil, nil
		}

		if err != nil {
			pageError(c, err)
			return
		}

		c.HTML(http.StatusOK, "subscription_confirm.html", web.SubscriptionConfirm(c, &userData, token, view, view != nil))
	})

	// The unsubscribe link in a mail is the one-click POST's URL, which a
	// browser opens with GET: it shows the subscriptions to end there.
	manage := func(c *gin.Context) {
		tokenPage(c)

		userData := auth.GetUserData(c)
		token := c.Param("manage_token")

		view, err := orExpired(subs.Manage(c, token))
		if err != nil {
			pageError(c, err)
			return
		}

		c.HTML(http.StatusOK, "subscriptions_manage.html", web.SubscriptionsManage(c, &userData, token, view, ""))
	}

	r.GET("/subscriptions/:manage_token", manage)
	r.GET("/subscriptions/:manage_token/unsubscribe/:subscription_id", manage)

	// What List-Unsubscribe-Post posts to, and what the manage page's
	// buttons post to: the token in the URL authorizes it and the answer is a
	// page, so it works from a mail client and from a browser alike.
	r.POST("/subscriptions/:manage_token/unsubscribe/:subscription_id", func(c *gin.Context) {
		tokenPage(c)

		userData := auth.GetUserData(c)
		token := c.Param("manage_token")

		err := subs.Unsubscribe(c, token, c.Param("subscription_id"))
		if errors.Is(err, service.ErrNotFound) {
			c.HTML(http.StatusOK, "subscriptions_manage.html", web.SubscriptionsManage(c, &userData, token, nil, ""))
			return
		}

		if err != nil {
			pageError(c, err)
			return
		}

		// the subscriber has no row left once they follow nobody
		view, err := orExpired(subs.Manage(c, token))
		if err != nil {
			pageError(c, err)
			return
		}

		c.HTML(http.StatusOK, "subscriptions_manage.html", web.SubscriptionsManage(c, &userData, token, view, unsubscribedNotice))
	})

	r.POST("/subscriptions/:manage_token/unsubscribe", func(c *gin.Context) {
		tokenPage(c)

		userData := auth.GetUserData(c)
		token := c.Param("manage_token")

		err := subs.UnsubscribeAll(c, token)
		if errors.Is(err, service.ErrNotFound) {
			c.HTML(http.StatusOK, "subscriptions_manage.html", web.SubscriptionsManage(c, &userData, token, nil, ""))
			return
		}

		if err != nil {
			pageError(c, err)
			return
		}

		c.HTML(http.StatusOK, "subscriptions_manage.html", web.SubscriptionsManage(c, &userData, token, nil, unsubscribedNotice))
	})

	actions.POST("/subscribers/:id/remove", requireUUIDParam("id"), jsonAction(d, func(c *gin.Context, u *model.User, _ struct{}) error {
		return subs.Remove(c, u, c.Param("id"))
	}))
}
