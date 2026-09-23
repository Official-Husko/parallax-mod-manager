package loverslab

import (
	"context"
	"fmt"
	"strconv"

	"golang.org/x/net/html"
)

// UnreadNotificationCount reads the real, current unread-notification count
// straight from the site's own bell badge:
//
//	<span class='ipsNotificationCount ipsHide' data-notificationType='notify' data-currentCount='0'>0</span>
//
// This badge is server-rendered on every real page, including this one - see
// docs/loverslab.md's Notifications section: the site's own live-updating bell
// (a JS controller polling an AJAX endpoint) is confirmed disabled server-side, so
// there is no real endpoint to poll instead of an ordinary page fetch. This is
// deliberately the only notification data this package reads - the notification
// list itself has never been observed populated (the test account has had zero
// notifications through every research pass), so its markup is unconfirmed and not
// guessed at here; a caller wanting the actual list opens /notifications/ in a
// browser instead (see LoversLabNotificationsURL).
func (c *Client) UnreadNotificationCount(ctx context.Context) (int, error) {
	doc, err := c.getDocument(ctx, BaseURL+"/notifications/")
	if err != nil {
		return 0, fmt.Errorf("checking notifications: %w", err)
	}
	return parseUnreadNotificationCount(doc), nil
}

// parseUnreadNotificationCount is UnreadNotificationCount's own parsing, pulled out
// so it can be tested directly against a fixture instead of a real request - see
// notifications_test.go. Note the attribute names below are lower-cased
// ("data-notificationtype", "data-currentcount") - golang.org/x/net/html lower-cases
// every attribute name while parsing, regardless of how the source HTML wrote it.
func parseUnreadNotificationCount(doc *html.Node) int {
	badge := findOne(doc, func(n *html.Node) bool {
		return isElement(n, "span") &&
			hasClass(n, "ipsNotificationCount") &&
			attrOr(n, "data-notificationtype") == "notify"
	})
	if badge == nil {
		return 0
	}
	n, _ := strconv.Atoi(attrOr(badge, "data-currentcount"))
	return n
}
