package secretbox

import (
	"os"
	"strings"
)

// machineIDFiles are where Linux keeps the id systemd (and dbus, on systems
// without it) generated when the machine was installed.
var machineIDFiles = []string{"/etc/machine-id", "/var/lib/dbus/machine-id"}

// machineID returns this computer's id, or "" when there is none.
func machineID() string {
	for _, path := range machineIDFiles {
		if data, err := os.ReadFile(path); err == nil {
			if id := strings.TrimSpace(string(data)); id != "" {
				return id
			}
		}
	}
	return ""
}
