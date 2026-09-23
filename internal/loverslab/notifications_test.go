package loverslab

import "testing"

// The confirmed real badge markup (docs/loverslab.md's Notifications section) - a
// real page always has this, zero or otherwise.
const zeroNotificationsFixture = `<html><body>
<a href='https://www.loverslab.com/notifications/' id='elFullNotifications' data-ipsTooltip title='Notifications' data-ipsMenu data-ipsMenu-closeOnClick='false'>
    <i class='fa fa-bell'></i> <span class='ipsNotificationCount ipsHide' data-notificationType='notify' data-currentCount='0'>0</span>
</a>
</body></html>`

func TestParseUnreadNotificationCountReadsZero(t *testing.T) {
	doc := parseFixture(t, zeroNotificationsFixture)
	if got := parseUnreadNotificationCount(doc); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestParseUnreadNotificationCountReadsANonZeroCount(t *testing.T) {
	fixture := `<html><body>
<a href='https://www.loverslab.com/notifications/' id='elFullNotifications' data-ipsMenu>
    <i class='fa fa-bell'></i> <span class='ipsNotificationCount' data-notificationType='notify' data-currentCount='3'>3</span>
</a>
</body></html>`
	doc := parseFixture(t, fixture)
	if got := parseUnreadNotificationCount(doc); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}

func TestParseUnreadNotificationCountHandlesMissingBadge(t *testing.T) {
	doc := parseFixture(t, `<html><body><p>nothing here</p></body></html>`)
	if got := parseUnreadNotificationCount(doc); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

// The inbox message badge shares the same class but a different
// data-notificationType - must not be confused with the notification one.
func TestParseUnreadNotificationCountIgnoresTheInboxBadge(t *testing.T) {
	fixture := `<html><body>
<span class='ipsNotificationCount' data-notificationType='inbox' data-currentCount='5'>5</span>
<span class='ipsNotificationCount' data-notificationType='notify' data-currentCount='2'>2</span>
</body></html>`
	doc := parseFixture(t, fixture)
	if got := parseUnreadNotificationCount(doc); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}
