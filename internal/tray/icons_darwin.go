package tray

import (
	"github.com/getlantern/systray"
	"kaffeinate/internal/artwork"
)

var (
	inactiveIcon = artwork.CupPNG(32, false)
	activeIcon   = artwork.CupPNG(32, true)
)

func setIcon(active bool) {
	iconBytes := inactiveIcon
	if active {
		iconBytes = activeIcon
	}
	systray.SetTemplateIcon(iconBytes, iconBytes)
}
