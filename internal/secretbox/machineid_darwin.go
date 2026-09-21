package secretbox

import (
	"os/exec"
	"strings"
)

// machineID returns the IOPlatformUUID macOS reports for this computer, or "" when
// it cannot be read.
func machineID() string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "IOPlatformUUID") {
			continue
		}
		// "IOPlatformUUID" = "XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"
		if _, after, ok := strings.Cut(line, "="); ok {
			return strings.Trim(strings.TrimSpace(after), `"`)
		}
	}
	return ""
}
