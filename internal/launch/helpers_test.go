package launch

import (
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// testClassicGame is a minimal classic-descriptor GameConfig fixture shared
// across this package's tests.
func testClassicGame() game.GameConfig {
	return game.GameConfig{
		Key:            "test-game",
		SteamAppID:     "281990",
		DescriptorType: mod.DescriptorClassic,
	}
}
