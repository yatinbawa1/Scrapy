package wallpaper

import (
	"fmt"
	"os/exec"
	"strings"
)

// SetWallpaper sets the given image on every desktop/display. On macOS a single
// picture is applied to all desktops via System Events.
func SetWallpaper(path string) error {
	escaped := strings.ReplaceAll(path, "'", "\\'")
	script := fmt.Sprintf(`tell application "System Events"
    set desktopCount to count of desktops
    repeat with i from 1 to desktopCount
        tell desktop i
            set picture to "%s"
        end tell
    end repeat
end tell`, escaped)

	cmd := exec.Command("osascript", "-e", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set wallpaper: %s: %w", string(output), err)
	}
	return nil
}

// SetWallpaperForDisplay sets the image on a single desktop (1-indexed).
func SetWallpaperForDisplay(path string, displayIndex int) error {
	script := fmt.Sprintf(`tell application "System Events"
    tell desktop %d
        set picture to "%s"
    end tell
end tell`, displayIndex, path)

	cmd := exec.Command("osascript", "-e", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set wallpaper display %d: %s: %w", displayIndex, string(output), err)
	}
	return nil
}
