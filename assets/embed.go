package assets

import (
	_ "embed"
	"os"
	"path/filepath"
)

// To update bundled APKs, download from the latest velo-platform release:
//   https://github.com/ril3y/velo-platform/releases/latest
// and replace the .apk files in this directory.

//go:embed velolauncher.apk
var embeddedVeloLauncher []byte

//go:embed serialbridge.apk
var embeddedSerialBridge []byte

//go:embed logo.png
var EmbeddedLogo []byte

// FindAPK returns a path to the APK, extracting from embedded data if needed.
// Checks for local file overrides first (next to exe or in assets/), then falls
// back to extracting the embedded APK to a temp file.
func FindAPK(name string) string {
	candidates := []string{name + ".apk"}

	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	// Check cwd, exe dir, and assets/ subdir
	for _, c := range candidates {
		for _, dir := range []string{".", exeDir, "assets", filepath.Join(exeDir, "assets")} {
			if dir == "" {
				continue
			}
			p := filepath.Join(dir, c)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// Fall back to embedded APK — write to temp file
	var data []byte
	switch name {
	case "velolauncher":
		data = embeddedVeloLauncher
	case "serialbridge":
		data = embeddedSerialBridge
	}
	if len(data) > 0 {
		tmp := filepath.Join(os.TempDir(), name+".apk")
		if err := os.WriteFile(tmp, data, 0644); err == nil {
			return tmp
		}
	}

	return ""
}
