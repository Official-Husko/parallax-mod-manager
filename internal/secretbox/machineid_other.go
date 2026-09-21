//go:build !linux && !windows && !darwin

package secretbox

// machineID has no source on this platform: ForThisMachine uses its fallback secret.
func machineID() string { return "" }
