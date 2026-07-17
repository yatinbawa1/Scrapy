package wallpaper

import (
	"fmt"
	"os/exec"
	"strings"
)

func SetWallpaper(path string) error {
	script := fmt.Sprintf(`tell application "System Events"
    set desktopCount to count of desktops
    repeat with i from 1 to desktopCount
        tell desktop i
            set picture to "%s"
        end tell
    end repeat
end tell`, strings.ReplaceAll(path, "'", "\\'"))

	cmd := exec.Command("osascript", "-e", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set wallpaper: %s: %w", string(output), err)
	}
	return nil
}

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
