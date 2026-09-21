package secretbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testKey = "SENTINELKEY0123456789ABCDEF012345"

func TestSealOpenRoundTrip(t *testing.T) {
	b, err := New([]byte("machine-a"))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal(testKey, "steam-api-key")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, testKey) {
		t.Errorf("the sealed value contains the plaintext: %q", sealed)
	}
	if !strings.HasPrefix(sealed, "v1.") {
		t.Errorf("sealed = %q, want the v1 prefix", sealed)
	}
	got, err := b.Open(sealed, "steam-api-key")
	if err != nil || got != testKey {
		t.Errorf("Open = %q, %v, want the key back", got, err)
	}
}

func TestSealIsRandomised(t *testing.T) {
	b, _ := New([]byte("machine-a"))
	a, _ := b.Seal(testKey, "p")
	c, _ := b.Seal(testKey, "p")
	if a == c {
		t.Error("two seals of the same value are identical, so the nonce is not fresh")
	}
}

func TestOpenFailsOnAnotherMachine(t *testing.T) {
	a, _ := New([]byte("machine-a"))
	b, _ := New([]byte("machine-b"))
	sealed, _ := a.Seal(testKey, "steam-api-key")
	if _, err := b.Open(sealed, "steam-api-key"); err != ErrCannotOpen {
		t.Errorf("Open on another machine = %v, want ErrCannotOpen", err)
	}
}

func TestOpenFailsForAnotherPurpose(t *testing.T) {
	b, _ := New([]byte("machine-a"))
	sealed, _ := b.Seal(testKey, "steam-api-key")
	if _, err := b.Open(sealed, "something-else"); err != ErrCannotOpen {
		t.Errorf("Open with another purpose = %v, want ErrCannotOpen", err)
	}
}

func TestOpenFailsOnTamperingAndJunk(t *testing.T) {
	b, _ := New([]byte("machine-a"))
	sealed, _ := b.Seal(testKey, "p")
	parts := strings.Split(sealed, ".")
	ct := []byte(parts[2])
	if ct[0] == 'A' {
		ct[0] = 'B'
	} else {
		ct[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(ct)
	for name, v := range map[string]string{
		"a flipped ciphertext character": tampered,
		"empty":                          "",
		"plain text":                     testKey,
		"wrong version":                  "v9." + parts[1] + "." + parts[2],
		"missing part":                   parts[0] + "." + parts[1],
		"bad base64":                     "v1.!!!.!!!",
		"short nonce":                    "v1.AAAA." + parts[2],
	} {
		if _, err := b.Open(v, "p"); err != ErrCannotOpen {
			t.Errorf("%s: Open = %v, want ErrCannotOpen", name, err)
		}
	}
}

func TestNewRejectsAnEmptySecret(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Error("New(nil) succeeded")
	}
}

func TestFingerprint(t *testing.T) {
	fp := Fingerprint(testKey)
	if len(fp) != 8 || strings.Contains(testKey, fp) {
		t.Errorf("Fingerprint = %q", fp)
	}
	if Fingerprint(testKey) != fp || Fingerprint(testKey+"x") == fp {
		t.Error("Fingerprint is not stable per key, or does not tell keys apart")
	}
}

func TestFallbackSecretIsCreatedOnceAndPrivate(t *testing.T) {
	dir := t.TempDir()
	first, err := fallbackSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fallbackSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("the fallback secret changed between calls")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "machine_secret.key"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("secret file mode = %v, want 0600", info.Mode().Perm())
		}
	}
	if _, err := fallbackSecret(""); err == nil {
		t.Error("a fallback secret with no folder succeeded")
	}
}

func TestForThisMachineRoundTrips(t *testing.T) {
	b, err := ForThisMachine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := b.Seal(testKey, "p")
	// A second Box for the same machine opens it: the derivation is deterministic.
	b2, err := ForThisMachine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if machineID() != "" {
		if got, err := b2.Open(sealed, "p"); err != nil || got != testKey {
			t.Errorf("a second Box on this machine could not open it: %q, %v", got, err)
		}
	}
}
