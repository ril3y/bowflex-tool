package device

import (
	"fmt"
	"strings"

	"github.com/ril3y/bowflex-tool/internal/adb"
)

// PreflightCheck queries the device for version info, platform key, and compatibility.
// Returns true if safe to proceed, false if blocked.
func PreflightCheck(ip string, log Logger) bool {
	log.Step("[PRE-FLIGHT] Device compatibility check")

	connect := func() (*adb.Conn, error) {
		return adb.Connect(ip, 5555, log.Warn)
	}

	// --- Device info (single batched shell command to avoid ADB staleness) ---
	conn, err := connect()
	if err != nil {
		log.Error(fmt.Sprintf("  Connection failed: %v", err))
		return false
	}
	propsRaw, _ := conn.Shell(
		"getprop ro.product.model; echo '|||'; " +
			"getprop ro.product.board; echo '|||'; " +
			"getprop ro.build.version.release; echo '|||'; " +
			"getprop ro.build.version.sdk; echo '|||'; " +
			"getprop ro.build.version.security_patch; echo '|||'; " +
			"getprop ro.serialno; echo '|||'; " +
			"getprop ro.build.fingerprint; echo '|||'; " +
			"uname -r; echo '|||'; " +
			"getprop ro.hardware; echo '|||'; " +
			"getprop ro.board.platform")
	conn.Close()

	propFields := strings.Split(propsRaw, "|||")
	getProp := func(i int) string {
		if i < len(propFields) {
			return strings.TrimSpace(propFields[i])
		}
		return ""
	}
	model := getProp(0)
	board := getProp(1)
	androidVer := getProp(2)
	sdkVer := getProp(3)
	secPatch := getProp(4)
	serial := getProp(5)
	fingerprint := getProp(6)
	kernel := getProp(7)
	chipset := getProp(8)
	soc := getProp(9)

	// Normalize Android version from SDK level
	if sdkVer == "28" || sdkVer == "9" {
		if sdkVer == "28" {
			androidVer = "9"
		}
	} else if secPatch == "9" || secPatch == "28" {
		androidVer = "9"
	}
	if androidVer != "9" {
		var sdkInt int
		fmt.Sscanf(sdkVer, "%d", &sdkInt)
		if sdkInt == 28 {
			androidVer = "9"
		}
	}

	// Board fallback: use ro.hardware or ro.board.platform if board is empty
	if board == "" {
		if chipset != "" {
			board = chipset
		} else if soc != "" {
			board = soc
		}
	}

	log.Info("  Device Information:")
	log.Dim(fmt.Sprintf("    Model: %s  Board: %s", model, board))
	log.Dim(fmt.Sprintf("    Android: %s (SDK %s)  Patch: %s", androidVer, sdkVer, secPatch))
	log.Dim(fmt.Sprintf("    Serial: %s", serial))
	log.Dim(fmt.Sprintf("    Kernel: %s", kernel))
	log.Dim(fmt.Sprintf("    Build: %s", fingerprint))

	// --- Nautilus package versions (single batched command) ---
	log.Info("  Package Versions:")
	conn, err = connect()
	if err != nil {
		log.Error(fmt.Sprintf("  Connection failed: %v", err))
		return false
	}

	nautilusPkgs := []struct{ pkg, label string }{
		{"com.nautilus.bowflex.usb", "JRNY"},
		{"com.nautilus.nautiluslauncher", "NautilusLauncher"},
		{"com.nautilus.g4assetmanager", "Asset Manager"},
		{"com.nautilus.nlssbcsystemsettings", "System Settings"},
		{"com.nautilus.nautilusfactoryreset", "Factory Reset"},
		{"com.nautilus.platform_hardwaretest", "Hardware Test"},
		{"com.nautilus.UtilityApp", "Utility App"},
		{"com.redbend.client", "Redbend OTA"},
	}

	var pkgCmds []string
	for _, p := range nautilusPkgs {
		pkgCmds = append(pkgCmds, fmt.Sprintf("dumpsys package %s 2>/dev/null | grep versionName | head -1", p.pkg))
	}
	pkgRaw, _ := conn.Shell(strings.Join(pkgCmds, "; echo '|||'; "))
	conn.Close()

	pkgResults := strings.Split(pkgRaw, "|||")
	jrnyVersion := ""
	launcherVersion := ""
	for i, p := range nautilusPkgs {
		ver := ""
		if i < len(pkgResults) {
			ver = strings.TrimSpace(pkgResults[i])
		}
		if idx := strings.Index(ver, "="); idx >= 0 {
			ver = ver[idx+1:]
		}
		if ver == "" {
			ver = "not installed"
		}
		log.Dim(fmt.Sprintf("    %s: %s", p.label, ver))
		if p.pkg == "com.nautilus.bowflex.usb" {
			jrnyVersion = ver
		}
		if p.pkg == "com.nautilus.nautiluslauncher" {
			launcherVersion = ver
		}
	}

	// --- Platform signing key verification ---
	log.Info("  Platform Key Check:")
	expectedKeySHA1 := "41791c9b8faf15e1acd5aaf59210fd42467d8277"

	certPkg := "com.nautilus.nautiluslauncher"
	if launcherVersion == "not installed" {
		certPkg = "com.nautilus.bowflex.usb"
	}
	conn, err = connect()
	if err != nil {
		log.Error(fmt.Sprintf("  Connection failed: %v", err))
		return false
	}
	certRaw, _ := conn.Shell(fmt.Sprintf(
		"dumpsys package %s 2>/dev/null | grep -iE 'signatures=|cert|sign|sha1' | head -10",
		certPkg))
	conn.Close()

	keyMatch := false
	certDump := strings.ToLower(strings.TrimSpace(certRaw))
	if strings.Contains(certDump, expectedKeySHA1) {
		keyMatch = true
		log.Success(fmt.Sprintf("    Platform key: MATCH (SHA-1: %s...)", expectedKeySHA1[:16]))
	} else {
		log.Dim(fmt.Sprintf("    Cert dump from %s: %.200s", certPkg, strings.TrimSpace(certRaw)))
		log.Warn("    Could not verify platform key via package dump")
		log.Dim("    Will verify during SerialBridge install (platform-signed APK)")
		log.Dim(fmt.Sprintf("    Expected SHA-1: %s", expectedKeySHA1))
	}

	// --- Compatibility verdict ---
	log.Info("")
	log.Info("  Compatibility Verdict:")

	blocked := false

	if androidVer != "9" {
		log.Error(fmt.Sprintf("    BLOCKED: Android %s — only Android 9 is supported", androidVer))
		blocked = true
	}

	boardLower := strings.ToLower(board)
	if strings.Contains(boardLower, "rk3") || strings.Contains(boardLower, "rockchip") || strings.Contains(boardLower, "nftm") {
		log.Success(fmt.Sprintf("    Board: %s", board))
	} else if board == "" {
		log.Warn("    Board not detected — could not determine hardware platform")
		log.Dim("    Proceeding anyway (model and packages will be checked)")
	} else {
		log.Error(fmt.Sprintf("    BLOCKED: Board '%s' — only Rockchip platforms are supported", board))
		blocked = true
	}

	if jrnyVersion == "not installed" {
		log.Error("    BLOCKED: JRNY not installed — is this a Bowflex bike?")
		blocked = true
	} else if jrnyVersion != "" {
		knownGood := "2.25.1"
		cmp := compareVersions(jrnyVersion, knownGood)
		if cmp == 0 {
			log.Success(fmt.Sprintf("    JRNY %s — exact match with tested version", jrnyVersion))
		} else if cmp < 0 {
			log.Warn(fmt.Sprintf("    JRNY %s — older than tested version (%s)", jrnyVersion, knownGood))
			log.Dim("    Likely compatible but not tested. Proceed with caution.")
		} else {
			log.Warn(fmt.Sprintf("    JRNY %s — NEWER than tested version (%s)", jrnyVersion, knownGood))
			log.Warn("    Nautilus may have changed internals. Proceed with caution.")
		}
	}

	if launcherVersion == "not installed" {
		log.Dim("    NautilusLauncher not installed (not needed — SerialBridge handles serial)")
	}

	if keyMatch {
		log.Success("    Platform key verified — SerialBridge will run as system")
	} else {
		log.Warn("    Platform key could not be verified pre-install")
		log.Dim("    If SerialBridge install fails, the key doesn't match and jailbreak is not possible")
	}

	if blocked {
		log.Error("")
		log.Error("  PRE-FLIGHT FAILED — Jailbreak blocked. Device is not compatible.")
		return false
	}

	log.Success("")
	log.Success("  PRE-FLIGHT PASSED — Device is compatible")
	log.Info("")
	return true
}

// compareVersions compares two dot-separated version strings.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		var ai, bi int
		if i < len(aParts) {
			fmt.Sscanf(aParts[i], "%d", &ai)
		}
		if i < len(bParts) {
			fmt.Sscanf(bParts[i], "%d", &bi)
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}
