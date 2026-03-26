package device

import (
	"fmt"
	"strings"

	"github.com/ril3y/bowflex-tool/internal/adb"
)

// FindAPKFunc is the function signature for locating APKs.
// Set by the caller to avoid circular dependency with assets package.
var FindAPKFunc func(name string) string

// RunJailbreak executes the full jailbreak sequence on the device.
func RunJailbreak(ip string, log Logger) {
	totalSteps := 9

	step := func(n int, name string) {
		log.Step(fmt.Sprintf("[STEP %d/%d] %s", n, totalSteps, name))
	}

	connect := func() (*adb.Conn, error) {
		return adb.Connect(ip, 5555, log.Warn)
	}

	// installAPK pushes and installs an APK by name. Returns true on success.
	installAPK := func(name, pkg string) bool {
		apkPath := FindAPKFunc(name)
		if apkPath == "" {
			log.Error(fmt.Sprintf("  %s.apk not found!", name))
			return false
		}
		log.Dim(fmt.Sprintf("  Using: %s", apkPath))

		// Uninstall old version first
		conn, err := connect()
		if err == nil {
			conn.Shell(fmt.Sprintf("am force-stop %s 2>/dev/null", pkg))
			conn.Shell(fmt.Sprintf("pm uninstall %s 2>/dev/null", pkg))
			conn.Close()
		}

		// Push APK
		conn, err = connect()
		if err != nil {
			log.Error(fmt.Sprintf("  Connection failed: %v", err))
			return false
		}
		tmpPath := fmt.Sprintf("/data/local/tmp/_%s.apk", name)
		err = conn.Push(apkPath, tmpPath, 0644)
		conn.Close()
		if err != nil {
			log.Error(fmt.Sprintf("  Push failed: %v", err))
			return false
		}

		// Install
		conn, err = connect()
		if err != nil {
			log.Error(fmt.Sprintf("  Connection failed: %v", err))
			return false
		}
		out, _ := conn.Shell(fmt.Sprintf("pm install -r %s 2>&1", tmpPath))
		conn.Close()
		outTrimmed := strings.TrimSpace(out)
		if !strings.Contains(out, "Success") {
			if outTrimmed == "" {
				outTrimmed = "(no output — install may have timed out)"
			}
			log.Error(fmt.Sprintf("  Install failed: %s", outTrimmed))
			return false
		}

		// Cleanup temp file
		conn, err = connect()
		if err == nil {
			conn.Shell(fmt.Sprintf("rm %s", tmpPath))
			conn.Close()
		}
		return true
	}

	// --- Step 1: Connect ---
	step(1, "Connecting to bike")
	log.Dim(fmt.Sprintf("  Target: %s:5555", ip))

	conn, err := connect()
	if err != nil {
		log.Error(fmt.Sprintf("  Connection failed: %v", err))
		log.Warn("  Make sure: 1) Bike is on same WiFi  2) Utility App was opened (9-tap)")
		return
	}
	log.Success("  Connected!")

	// --- Step 2: Verify ---
	step(2, "Verifying ADB shell")
	out, err := conn.Shell("id")
	if err != nil {
		log.Error(fmt.Sprintf("  Shell failed: %v", err))
		conn.Close()
		return
	}
	idStr := strings.TrimSpace(out)
	if idx := strings.Index(idStr, " groups="); idx > 0 {
		idStr = idStr[:idx]
	}
	log.Success(fmt.Sprintf("  %s", idStr))
	conn.Close()

	// Pre-flight already passed (enforced by UI). From here we modify the device.
	// If anything fails, auto-restore to stock.
	failed := false
	defer func() {
		if failed {
			log.Step("")
			log.Error("=== JAILBREAK FAILED — RESTORING STOCK ===")
			log.Warn("  A step failed. Automatically restoring bike to stock state...")
			log.Info("")
			RunRestore(ip, log)
		}
	}()

	fail := func(msg string) {
		log.Error(msg)
		failed = true
	}

	// --- Step 3: Disable JRNY + AppMonitor ---
	step(3, "Disabling JRNY and AppMonitor")
	conn, err = connect()
	if err != nil {
		fail(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	conn.Shell("am stop-service com.nautilus.nautiluslauncher/.thirdparty.appmonitor.AppMonitorService 2>/dev/null")
	conn.Shell("pm disable-user --user 0 com.nautilus.bowflex.usb 2>/dev/null")
	conn.Shell("pm disable-user --user 0 com.nautilus.g4assetmanager 2>/dev/null")
	conn.Shell("pm disable-user --user 0 com.nautilus.nlssbcsystemsettings 2>/dev/null")
	conn.Shell("pm disable-user --user 0 com.redbend.vdmc 2>/dev/null")
	conn.Shell("pm disable-user --user 0 com.redbend.client 2>/dev/null")
	conn.Shell("pm disable-user --user 0 com.redbend.dualpart.service.app 2>/dev/null")
	conn.Shell("settings put global software_update 0 2>/dev/null")
	conn.Shell("settings put global auto_update 0 2>/dev/null")
	conn.Close()
	log.Success("  JRNY, AppMonitor, OTA all disabled")

	// --- Step 4: Check current state ---
	step(4, "Checking current state")
	conn, err = connect()
	if err != nil {
		fail(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	out, _ = conn.Shell("pm list packages 2>/dev/null | grep -iE 'freewheel.launcher|freewheel.bridge'")
	conn.Close()

	if strings.Contains(out, "freewheel.launcher") {
		log.Dim("  VeloLauncher present -- will reinstall")
	}
	if strings.Contains(out, "freewheel.bridge") {
		log.Dim("  SerialBridge present -- will reinstall")
	}

	// --- Step 5: Install SerialBridge (critical) ---
	step(5, "Installing SerialBridge (takes over serial port)")
	if !installAPK("serialbridge", "io.freewheel.bridge") {
		fail("  SerialBridge install failed — cannot proceed without serial port control")
		return
	}
	log.Success("  SerialBridge installed (io.freewheel.bridge)")
	conn, err = connect()
	if err == nil {
		conn.Shell("am force-stop com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("pm disable-user --user 0 com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("am start-foreground-service -n io.freewheel.bridge/.BridgeService 2>/dev/null")
		conn.Close()
		log.Success("  NautilusLauncher disabled, SerialBridge started on /dev/ttyS4")
	}

	// --- Step 6: Install VeloLauncher (critical) ---
	step(6, "Installing VeloLauncher")
	if !installAPK("velolauncher", "io.freewheel.launcher") {
		fail("  VeloLauncher install failed — cannot proceed without a launcher")
		return
	}
	log.Success("  VeloLauncher installed")
	conn, err = connect()
	if err == nil {
		conn.Shell("cmd package set-home-activity io.freewheel.launcher/.MainActivity")
		conn.Shell("pm disable-user --user 0 com.android.launcher3 2>/dev/null")
		conn.Shell("pm disable-user --user 0 com.android.quickstep 2>/dev/null")
		conn.Shell("appops set io.freewheel.launcher SYSTEM_ALERT_WINDOW allow")
		conn.Shell("pm grant io.freewheel.launcher android.permission.WRITE_SETTINGS 2>/dev/null")
		conn.Close()
		log.Success("  Set as default home, other launchers disabled")
		log.Dim("  Overlay + settings permissions granted")
	}

	// --- Step 7: Apply system settings ---
	step(7, "Applying system settings")
	conn, err = connect()
	if err == nil {
		// Disable Google apps (2GB device, GMS crashes BT/WiFi)
		googleApps := []struct{ pkg, name string }{
			{"com.google.android.gms", "Play Services"},
			{"com.google.android.gsf", "Google Services Framework"},
			{"com.android.chrome", "Chrome"},
		}
		for _, a := range googleApps {
			conn.Shell(fmt.Sprintf("pm disable-user --user 0 %s 2>/dev/null", a.pkg))
			log.Dim(fmt.Sprintf("  Disabled: %s", a.name))
		}
		conn.Shell("pm enable com.android.vending 2>/dev/null")
		log.Success("  Play Store kept enabled")
		conn.Shell("pm enable com.google.android.webview 2>/dev/null")
		log.Success("  WebView enabled")

		// Enable navbar, disable kiosk mode
		conn.Shell("settings put secure navigationbar_switch 1")
		conn.Shell("settings put global force_show_navbar 1")
		conn.Shell("settings put secure statusbar_switch 0")
		conn.Shell("settings put secure notification_switch 0")
		conn.Shell("settings put secure ntls_launcher_preference 0")
		conn.Shell("settings put global stay_on_while_plugged_in 3")
		log.Dim("  Navbar enabled, screen stays on while plugged in")
		conn.Close()
	}

	// ADB persistence — persist props survive reboots
	conn, err = connect()
	if err == nil {
		conn.Shell("setprop persist.sys.usb.config adb")
		conn.Shell("setprop persist.adb.tcp.port 5555")
		conn.Shell("settings put global adb_enabled 1")
		conn.Close()
		log.Dim("  ADB persistence set (persist.sys.usb.config, persist.adb.tcp.port)")
	}

	// --- Step 9: Final setup ---
	step(9, "Final setup")

	conn, err = connect()
	if err == nil {
		conn.Shell("input keyevent KEYCODE_HOME")
		conn.Close()
		log.Success("  Home screen active")
	}

	// Check SerialBridge TCP:9999
	conn, err = connect()
	if err == nil {
		tcpCheck, _ := conn.Shell("nc -z localhost 9999 2>/dev/null && echo ALIVE || echo DEAD")
		conn.Close()
		if strings.Contains(tcpCheck, "ALIVE") {
			log.Success("  TCP:9999 alive (SerialBridge serving sensor data)")
		} else {
			log.Dim("  TCP:9999 not yet listening (SerialBridge may need a moment)")
		}
	}

	// Final summary
	log.Step("")
	log.Success("=== JAILBREAK COMPLETE ===")
	log.Info("")
	log.Dim("  SerialBridge owns /dev/ttyS4, serves TCP:9999 (NautilusLauncher disabled)")
	log.Dim("  VeloLauncher set as home screen (free, no subscription)")
	log.Dim("  JRNY, AppMonitor, OTA all disabled (persists across reboots)")
	log.Dim("  Google apps disabled (2GB RAM), Play Store + WebView enabled")
	log.Dim("  ADB on port 5555, navbar enabled, kiosk mode off")
	log.Dim("  Screen stays on while plugged in")
	log.Info("")
	log.Info("VeloLauncher BootReceiver restarts itself + SerialBridge on reboot.")
	log.Info("All package disabling persists natively across Android reboots.")
}
