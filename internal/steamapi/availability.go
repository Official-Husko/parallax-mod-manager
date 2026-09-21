package steamapi

// Availability is what a Workshop item's state means for someone who has it
// installed: whether it can still be found and updated. It is worked out from
// what Steam answered (see Classify), so a mod can be marked unlisted, private or
// deleted without the caller knowing which API answered.
type Availability string

const (
	// AvailabilityUnknown means nothing can be said: Steam was unwell, or the
	// answer does not say. Never shown as a flag.
	AvailabilityUnknown Availability = ""
	// AvailabilityPublic is an ordinary, listed item.
	AvailabilityPublic Availability = "public"
	// AvailabilityUnlisted is an item reachable by its link but absent from the
	// Workshop's search and listings. It works in the game and can update.
	AvailabilityUnlisted Availability = "unlisted"
	// AvailabilityPrivate is an item its author restricted: private, or friends only.
	AvailabilityPrivate Availability = "private"
	// AvailabilityDeleted is an item that is gone from the Workshop (deleted by its
	// author, or removed by Steam's moderators). An installed copy keeps working but
	// will never update.
	AvailabilityDeleted Availability = "deleted"
)

// Why Classify decided what it did - the frontend words each one, so the
// explanation always says how sure the app is.
const (
	// Steam's own record says so (visibility, or the ItemDeleted result).
	ReasonRecord = "record"
	// Friends-only visibility (a kind of private).
	ReasonFriends = "friends"
	// Steam answered "access denied" for the item.
	ReasonDenied = "denied"
	// Moderators banned it.
	ReasonBanned = "banned"
	// The free API does not return it but its Workshop page is up: what an unlisted
	// item looks like without a key.
	ReasonPageUp = "page-up"
	// The free API does not return it and its Workshop page is gone: deleted, or made
	// private (an anonymous page cannot tell the two apart).
	ReasonPageGone = "page-gone"
)

// Standing is Classify's answer: what became of an item and why the app thinks so.
type Standing struct {
	State  Availability
	Reason string
}

// Classify says what became of one Workshop item from what Steam answered for
// it. live and checked describe its Workshop page, which is only looked at for an
// item Steam answered "not found" for (the free API says that for items whose page
// is up, so on its own it proves nothing): checked is false when the page was not
// or could not be looked at, and the item is then left Unknown rather than guessed.
//
// The keyed API's record is the most exact source: it carries the visibility
// (0 public, 1 friends only, 2 private, 3 unlisted) and can say ItemDeleted. The
// free API only says "not found", so there the page decides: up means the item is
// there but unlisted, gone means it is deleted or private.
func Classify(d PublishedFileDetails, live, checked bool) Standing {
	r := EResult(d.Result)
	switch {
	case d.Banned:
		return Standing{AvailabilityDeleted, ReasonBanned}
	case r.IsDeleted():
		return Standing{AvailabilityDeleted, ReasonRecord}
	case r.IsOK():
		switch d.Visibility {
		case 1:
			return Standing{AvailabilityPrivate, ReasonFriends}
		case 2:
			return Standing{AvailabilityPrivate, ReasonRecord}
		case 3:
			return Standing{AvailabilityUnlisted, ReasonRecord}
		}
		return Standing{AvailabilityPublic, ""}
	case r == ResultAccessDenied || r == ResultInsufficientRights:
		return Standing{AvailabilityPrivate, ReasonDenied}
	case r.IsNotFound():
		if !checked {
			return Standing{}
		}
		if live {
			return Standing{AvailabilityUnlisted, ReasonPageUp}
		}
		return Standing{AvailabilityDeleted, ReasonPageGone}
	}
	return Standing{}
}

// NeedsPageCheck says whether an item's own Workshop page has to be looked at to
// know what became of it: Steam answered something other than success, and not with
// a verdict of its own (banned, deleted, access denied) or a sign that Steam itself
// was unwell (a timeout, a busy service, a rate limit - see EResult). "Not found" is
// the case this is for: the free API says it for unlisted items whose page is up, so
// it proves nothing on its own.
func NeedsPageCheck(d PublishedFileDetails) bool {
	r := EResult(d.Result)
	return !d.Banned && !r.IsOK() && !r.IsDeleted() && !r.IsTransient() && !r.IsRateLimit() &&
		r != ResultAccessDenied && r != ResultInsufficientRights
}
