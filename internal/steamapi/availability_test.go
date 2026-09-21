package steamapi

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name          string
		d             PublishedFileDetails
		live, checked bool
		want          Standing
	}{
		// What the keyed API returned for the real unlisted item 2780180614.
		{"keyed record, unlisted", PublishedFileDetails{Result: 1, Visibility: 3}, false, false, Standing{AvailabilityUnlisted, ReasonRecord}},
		{"keyed record, private", PublishedFileDetails{Result: 1, Visibility: 2}, false, false, Standing{AvailabilityPrivate, ReasonRecord}},
		{"keyed record, friends only", PublishedFileDetails{Result: 1, Visibility: 1}, false, false, Standing{AvailabilityPrivate, ReasonFriends}},
		{"an ordinary public item", PublishedFileDetails{Result: 1, Visibility: 0}, false, false, Standing{AvailabilityPublic, ""}},
		{"a free-API item (visibility 0)", PublishedFileDetails{Result: 1}, false, false, Standing{AvailabilityPublic, ""}},
		{"banned by moderators", PublishedFileDetails{Result: 1, Banned: true}, false, false, Standing{AvailabilityDeleted, ReasonBanned}},
		{"ItemDeleted from the keyed API", PublishedFileDetails{Result: 86}, false, false, Standing{AvailabilityDeleted, ReasonRecord}},
		{"access denied", PublishedFileDetails{Result: 15}, false, false, Standing{AvailabilityPrivate, ReasonDenied}},
		{"insufficient privilege", PublishedFileDetails{Result: 24}, false, false, Standing{AvailabilityPrivate, ReasonDenied}},
		{"not found, page up (the free API's view of an unlisted item)", PublishedFileDetails{Result: 9}, true, true, Standing{AvailabilityUnlisted, ReasonPageUp}},
		{"not found, page gone", PublishedFileDetails{Result: 9}, false, true, Standing{AvailabilityDeleted, ReasonPageGone}},
		{"no match, page gone", PublishedFileDetails{Result: 42}, false, true, Standing{AvailabilityDeleted, ReasonPageGone}},
		{"not found, page not checked", PublishedFileDetails{Result: 9}, false, false, Standing{}},
		{"not found, the page said nothing certain", PublishedFileDetails{Result: 9}, true, false, Standing{}},
		{"a busy Steam", PublishedFileDetails{Result: 10}, false, false, Standing{}},
		{"a rate limit", PublishedFileDetails{Result: 84}, false, false, Standing{}},
		{"an unrelated code", PublishedFileDetails{Result: 8}, false, false, Standing{}},
	}
	for _, tc := range cases {
		if got := Classify(tc.d, tc.live, tc.checked); got != tc.want {
			t.Errorf("%s: Classify = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestNeedsPageCheckOnlyForNotFound(t *testing.T) {
	for code, want := range map[int]bool{1: false, 9: true, 42: true, 15: false, 24: false, 86: false, 10: false, 16: false, 84: false, 8: true} {
		if got := NeedsPageCheck(PublishedFileDetails{Result: code}); got != want {
			t.Errorf("result %d: NeedsPageCheck = %v, want %v", code, got, want)
		}
	}
	if NeedsPageCheck(PublishedFileDetails{Result: 9, Banned: true}) {
		t.Error("a banned item needs no page check")
	}
}
