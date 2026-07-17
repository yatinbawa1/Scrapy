package wallpaper

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	spiSetDesktopWallpaper = 0x0014
	spiUpdateIniFile       = 0x0001
	spiSendChange          = 0x0002
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
)

// SetWallpaper sets the image across all monitors. It configures the
// "Span" wallpaper style so a single image covers every display, writes the
// path to the registry, and notifies the system.
func SetWallpaper(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve wallpaper path: %w", err)
	}

	// Span across all monitors.
	if err := setRegistry("WallpaperStyle", "10"); err != nil {
		return err
	}
	if err := setRegistry("TileWallpaper", "0"); err != nil {
		return err
	}
	if err := setRegistry("Wallpaper", abs); err != nil {
		return err
	}

	wide, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return fmt.Errorf("wallpaper path encoding: %w", err)
	}

	r, _, callErr := procSystemParametersInfoW.Call(
		uintptr(spiSetDesktopWallpaper),
		uintptr(0),
		uintptr(unsafe.Pointer(wide)),
		uintptr(spiUpdateIniFile|spiSendChange),
	)
	if r == 0 {
		if callErr != nil {
			return fmt.Errorf("SystemParametersInfoW failed: %w", callErr)
		}
		return fmt.Errorf("SystemParametersInfoW failed with no error code")
	}
	return nil
}

// SetWallpaperForDisplay is not individually addressable through the simple
// SystemParametersInfo API on Windows. We fall back to spanning the image
// across all monitors, which covers the requested display.
func SetWallpaperForDisplay(path string, displayIndex int) error {
	return SetWallpaper(path)
}

func setRegistry(key, value string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Control Panel\Desktop`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open wallpaper registry key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(key, value); err != nil {
		return fmt.Errorf("set registry %s: %w", key, err)
	}
	return nil
}
