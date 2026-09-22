package loverslab

import "testing"

const loginFormFixture = `<form accept-charset='utf-8' method='post' action='https://www.loverslab.com/login/' class='ipsBox_alt'>
  <input type="hidden" name="csrfKey" value="4ca509366a475a48b16d72038234199b">
  <input type="email" name="auth" id="auth" autocomplete="email">
  <input type="password" name="password" id="password" autocomplete="current-password">
</form>`

func TestExtractCSRFKey(t *testing.T) {
	key, ok := extractCSRFKey([]byte(loginFormFixture))
	if !ok {
		t.Fatal("expected to find a csrfKey")
	}
	if key != "4ca509366a475a48b16d72038234199b" {
		t.Errorf("key = %q, want 4ca509366a475a48b16d72038234199b", key)
	}
}

func TestExtractCSRFKeyMissing(t *testing.T) {
	if _, ok := extractCSRFKey([]byte("<html><body>no csrfKey field on this page</body></html>")); ok {
		t.Error("expected extractCSRFKey to fail when there is no csrfKey field")
	}
}

func TestExtractCSRFKeyAcceptsDoubleOrSingleQuotes(t *testing.T) {
	if _, ok := extractCSRFKey([]byte(`<input type="hidden" name="csrfKey" value="abc123">`)); !ok {
		t.Error("double-quoted csrfKey not matched")
	}
	if _, ok := extractCSRFKey([]byte(`<input type='hidden' name='csrfKey' value='abc123'>`)); !ok {
		t.Error("single-quoted csrfKey not matched")
	}
}

func TestIsLoggedInAndMemberIDBeforeLogin(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.IsLoggedIn() {
		t.Error("a fresh client should not report as logged in")
	}
	if id := c.MemberID(); id != "" {
		t.Errorf("MemberID on a fresh client = %q, want empty", id)
	}
}

func TestIsLoggedInAfterImportingASession(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.ImportSession(`[{"name":"ips4_member_id","value":"98765"},{"name":"ips4_login_key","value":"secret"}]`); err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	if !c.IsLoggedIn() {
		t.Error("expected IsLoggedIn to be true once ips4_member_id is present")
	}
	if got := c.MemberID(); got != "98765" {
		t.Errorf("MemberID = %q, want 98765", got)
	}
}
