package steamapi

import (
	"strings"
	"testing"
)

func TestEResultTableIsComplete(t *testing.T) {
	// The Steamworks table lists 1-108 with 4 unused; every named constant this
	// package uses must be in it with the documented name.
	want := map[EResult]string{
		ResultOK: "OK", ResultFail: "Fail", ResultFileNotFound: "FileNotFound", ResultBusy: "Busy",
		ResultAccessDenied: "AccessDenied", ResultTimeout: "Timeout", ResultBanned: "Banned",
		ResultServiceUnavailable: "ServiceUnavailable", ResultInsufficientRights: "InsufficientPrivilege",
		ResultLimitExceeded: "LimitExceeded", ResultConnectFailed: "ConnectFailed", ResultNoMatch: "NoMatch",
		ResultRemoteCallFailed: "RemoteCallFailed", ResultRateLimitExceeded: "RateLimitExceeded",
		ResultItemDeleted: "ItemDeleted", ResultNoConnection: "NoConnection",
	}
	for code, name := range want {
		if got := code.Name(); got != name {
			t.Errorf("EResult(%d).Name() = %q, want %q", int(code), got, name)
		}
	}
	if n := len(eresults); n != 107 {
		t.Errorf("table has %d entries, want the 107 Steamworks documents", n)
	}
	for code, info := range eresults {
		if info.name == "" || info.text == "" {
			t.Errorf("EResult(%d) has an empty name or description", int(code))
		}
	}
}

func TestDescribeResult(t *testing.T) {
	if got := EResult(9).String(); got != "9 (FileNotFound)" {
		t.Errorf("String() = %q", got)
	}
	if got := DescribeResult(9); !strings.HasPrefix(got, "9 (FileNotFound): ") {
		t.Errorf("DescribeResult(9) = %q", got)
	}
	if got := EResult(4242).String(); got != "4242 (Unknown)" {
		t.Errorf("an unlisted code = %q", got)
	}
	if EResult(4242).Description() == "" {
		t.Error("an unlisted code has no description")
	}
}

func TestEResultClasses(t *testing.T) {
	cases := []struct {
		code                                           int
		ok, notFound, deleted, denied, rate, transient bool
	}{
		{1, true, false, false, false, false, false},
		{9, false, true, false, false, false, false},
		{42, false, true, false, false, false, false},
		{86, false, false, true, false, false, false},
		{15, false, false, false, true, false, false},
		{24, false, false, false, true, false, false},
		{17, false, false, false, true, false, false},
		{25, false, false, false, false, true, false},
		{84, false, false, false, false, true, false},
		{2, false, false, false, false, false, true},
		{3, false, false, false, false, false, true},
		{10, false, false, false, false, false, true},
		{16, false, false, false, false, false, true},
		{20, false, false, false, false, false, true},
		{35, false, false, false, false, false, true},
		{55, false, false, false, false, false, true},
		{8, false, false, false, false, false, false},
	}
	for _, tc := range cases {
		r := EResult(tc.code)
		got := [6]bool{r.IsOK(), r.IsNotFound(), r.IsDeleted(), r.IsDenied(), r.IsRateLimit(), r.IsTransient()}
		want := [6]bool{tc.ok, tc.notFound, tc.deleted, tc.denied, tc.rate, tc.transient}
		if got != want {
			t.Errorf("EResult(%d) classes = %v, want %v (ok, notFound, deleted, denied, rateLimit, transient)", tc.code, got, want)
		}
	}
}
