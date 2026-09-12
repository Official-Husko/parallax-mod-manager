package xhash

import "testing"

// Golden-constant tests: fixed input -> fixed expected output. These exist
// specifically to catch a future accidental swap to a random-seeded hash
// (e.g. hash/maphash) for something that must stay stable across process
// restarts and across machines, per docs/performance-strategy.md.
func TestBytesGoldenValue(t *testing.T) {
	got := Bytes([]byte("parallax-mod-manager xhash golden test"))
	const want uint64 = 2886152899554806087
	if got != want {
		t.Errorf("Bytes(...) = %d, want %d (hash algorithm or seed changed?)", got, want)
	}
}

func TestBytesEmptyInputGoldenValue(t *testing.T) {
	got := Bytes(nil)
	const want uint64 = 17241709254077376921
	if got != want {
		t.Errorf("Bytes(nil) = %d, want %d", got, want)
	}
}

func TestDefinitionMatchesBytesAlgorithm(t *testing.T) {
	// Definition and Bytes are documented to share the same underlying
	// algorithm (only their call-site meaning differs) - verify that holds.
	data := []byte("common/buildings entry content")
	if Definition(data) != Bytes(data) {
		t.Errorf("Definition and Bytes diverged for identical input")
	}
}

func TestDeterministicAcrossCalls(t *testing.T) {
	data := []byte("determinism check")
	first := Bytes(data)
	for i := 0; i < 5; i++ {
		if got := Bytes(data); got != first {
			t.Fatalf("call %d: Bytes(...) = %d, want %d (not deterministic)", i, got, first)
		}
	}
}

func TestDifferentInputsDifferentHashes(t *testing.T) {
	a := Bytes([]byte("content A"))
	b := Bytes([]byte("content B"))
	if a == b {
		t.Errorf("expected different hashes for different content, both = %d", a)
	}
}
