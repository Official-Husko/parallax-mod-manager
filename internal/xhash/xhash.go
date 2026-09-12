// Package xhash is the single place allowed to compute content hashes used
// for change-detection and conflict-identity comparison across this project.
// It exists so nothing else ever reaches for Go's hash/maphash - its seed is
// random per process, which silently breaks any hash persisted to disk and
// compared across app restarts (see docs/performance-strategy.md).
//
// xxHash is a fast, fixed-seed, non-cryptographic hash: exactly what's needed
// here, since there's no adversary to defend against - only "did this content
// change" and "do two mods define this object identically".
package xhash

import "github.com/cespare/xxhash/v2"

// Bytes hashes raw file content, for cache invalidation (has this file's
// content changed since it was last read?).
func Bytes(data []byte) uint64 {
	return xxhash.Sum64(data)
}

// Definition hashes normalized definition content, for conflict-identity
// comparison (do two mods' versions of the same object have identical
// content, once insignificant formatting differences are stripped?). Callers
// are expected to pass already-normalized bytes (see internal/definition).
func Definition(normalized []byte) uint64 {
	return xxhash.Sum64(normalized)
}
