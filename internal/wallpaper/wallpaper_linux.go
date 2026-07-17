package wallpaper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SetWallpaper sets the image across all monitors. It probes the running
// desktop environment and uses the appropriate native mechanism, falling back
// through several common Linux desktop environments.
func SetWallpaper(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve wallpaper path: %w", err)
	}
	uri := "file://" + abs

	desktops := detectDesktops()

	var lastErr error
	for _, de := range desktops {
		switch de {
		case "gnome", "unity", "cinnamon", "pop":
			// gsettings applies the wallpaper to every monitor at once.
			lastErr = run("gsettings", "set", "org.gnome.desktop.background", "picture-uri", uri)
			if lastErr == nil {
				run("gsettings", "set", "org.gnome.desktop.background", "picture-uri-dark", uri)
				return nil
			}
		case "kde":
			// Plasma: set the wallpaper on all desktops/activities via script.
			script := fmt.Sprintf(`
var allDesktops = desktops();
for (var i = 0; i < allDesktops.length; i++) {
    d = allDesktops[i];
    d.wallpaperPlugin = "org.kde.image";
    d.currentConfigGroup = Array("Wallpaper", "org.kde.image", "General");
    d.writeConfig("Image", "file://%s");
}`, abs)
			lastErr = run("qdbus", "org.kde.plasmashell", "/PlasmaShell", "org.kde.plasmashell.evaluateScript", script)
			if lastErr == nil {
				return nil
			}
		case "xfce":
			lastErr = run("xfconf-query", "-c", "xfce4-desktop",
				"-p", "/backdrop/screen0/monitor0/workspace0/last-image", "-s", abs)
			if lastErr == nil {
				return nil
			}
		case "lxde":
			lastErr = run("pcmanfm", "--set-wallpaper", abs, "--wallpaper-mode", "fit")
			if lastErr == nil {
				return nil
			}
		case "feh":
			// feh --bg-fill sets the same image on every connected monitor.
			lastErr = run("feh", "--bg-fill", abs)
			if lastErr == nil {
				return nil
			}
		}
	}

	if lastErr != nil {
		return fmt.Errorf("set wallpaper: no supported desktop environment found: %w", lastErr)
	}
	return fmt.Errorf("set wallpaper: no supported desktop environment found")
}

// SetWallpaperForDisplay sets the image on a single monitor. On Linux this is
// only cleanly supported by some environments; when per-monitor control is not
// available we fall back to setting the wallpaper on all monitors.
func SetWallpaperForDisplay(path string, displayIndex int) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve wallpaper path: %w", err)
	}
	de := primaryDesktop()
	switch de {
	case "gnome", "unity", "cinnamon", "pop":
		uri := "file://" + abs
		if err := run("gsettings", "set", "org.gnome.desktop.background", "picture-uri", uri); err != nil {
			return err
		}
		return run("gsettings", "set", "org.gnome.desktop.background", "picture-uri-dark", uri)
	case "kde":
		script := fmt.Sprintf(`
var d = desktopForScreen(%d);
if (d) {
    d.wallpaperPlugin = "org.kde.image";
    d.currentConfigGroup = Array("Wallpaper", "org.kde.image", "General");
    d.writeConfig("Image", "file://%s");
}`, displayIndex-1, abs)
		return run("qdbus", "org.kde.plasmashell", "/PlasmaShell", "org.kde.plasmashell.evaluateScript", script)
	case "xfce":
		return run("xfconf-query", "-c", "xfce4-desktop",
			"-p", fmt.Sprintf("/backdrop/screen0/monitor%d/workspace0/last-image", displayIndex-1), "-s", abs)
	case "lxde":
		return run("pcmanfm", "--set-wallpaper", abs, "--wallpaper-mode", "fit")
	default:
		// feh and unknown environments: apply to all monitors.
		return SetWallpaper(abs)
	}
}

// detectDesktops returns an ordered list of candidate desktop environments to
// try, with the detected one first.
func detectDesktops() []string {
	de := primaryDesktop()
	order := []string{de}
	candidates := []string{"gnome", "kde", "xfce", "cinnamon", "unity", "pop", "lxde", "feh"}
	for _, c := range candidates {
		if c != de {
			order = append(order, c)
		}
	}
	return order
}

func primaryDesktop() string {
	if v := os.Getenv("XDG_CURRENT_DESKTOP"); v != "" {
		lower := strings.ToLower(v)
		for _, name := range []string{"gnome", "unity", "cinnamon", "pop", "kde", "xfce", "lxde", "mate", "budgie"} {
			if strings.Contains(lower, name) {
				return name
			}
		}
	}
	if v := os.Getenv("DESKTOP_SESSION"); v != "" {
		lower := strings.ToLower(v)
		for _, name := range []string{"gnome", "kde", "xfce", "lxde", "mate"} {
			if strings.Contains(lower, name) {
				return name
			}
		}
	}
	if _, err := exec.LookPath("gsettings"); err == nil {
		return "gnome"
	}
	if _, err := exec.LookPath("qdbus"); err == nil {
		return "kde"
	}
	if _, err := exec.LookPath("xfconf-query"); err == nil {
		return "xfce"
	}
	if _, err := exec.LookPath("pcmanfm"); err == nil {
		return "lxde"
	}
	if _, err := exec.LookPath("feh"); err == nil {
		return "feh"
	}
	return "gnome"
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s: %w", name, strings.TrimSpace(string(output)), err)
	}
	return nil
}
