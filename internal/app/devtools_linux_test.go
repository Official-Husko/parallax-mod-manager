package app

import (
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// Developer tools exist in development builds only: elsewhere the setting is ignored, even when a
// settings file has it switched on.
func TestDeveloperToolsStatus(t *testing.T) {
	a := &App{preferences: preferences.Defaults()}
	st := a.DeveloperToolsStatus()
	if st.Available != devBuild || st.Enabled || st.OS != "linux" {
		t.Errorf("fresh: %+v", st)
	}

	a.preferences.DeveloperTools = true
	st = a.DeveloperToolsStatus()
	if st.Enabled != devBuild {
		t.Errorf("setting on: %+v (it counts only in a development build)", st)
	}
	a.preferences.DeveloperTools = false
	if st = a.DeveloperToolsStatus(); st.Enabled {
		t.Errorf("setting off: %+v", st)
	}
}

func TestEnvironmentReportMentionsDeveloperToolsOnlyInADevelopmentBuild(t *testing.T) {
	a := &App{preferences: preferences.Defaults()}
	joined := strings.Join(a.environmentReportLines(), "\n")
	if got := strings.Contains(joined, "developer tools off"); got != devBuild {
		t.Errorf("report mentions developer tools = %v, want %v (dev build):\n%s", got, devBuild, joined)
	}
}
