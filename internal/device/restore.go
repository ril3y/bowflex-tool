package device

import (
	"fmt"
	"strings"
	"time"

	"github.com/ril3y/bowflex-tool/internal/adb"
)

// RunRestore reverts the device to stock configuration.
func RunRestore(ip string, log Logger) {
	totalSteps := 8

	step := func(n int, name string) {
		log.Step(fmt.Sprintf("[STEP %d/%d] %s", n, totalSteps, name))
	}

	connect := func() (*adb.Conn, error) {
		return adb.Connect(ip, 5555, log.Warn)
	}

	// --- Step 1: Connect ---
	step(1, "Connecting to bike")
	conn, err := connect()
	if err != nil {
		log.Error(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	log.Success("  Connected!")
	conn.Close()

	// --- Step 2: Restore stock packages and settings ---
	step(2, "Restoring stock packages and settings")
	conn, err = connect()
	if err == nil {
		stockPkgs := []string{
			"com.nautilus.bowflex.usb",
			"com.nautilus.nautiluslauncher",
			"com.redbend.client", "com.redbend.vdmc", "com.redbend.dualpart.service.app",
			"com.nautilus.g4assetmanager", "com.nautilus.nlssbcsystemsettings",
		}
		for _, pkg := range stockPkgs {
			conn.Shell(fmt.Sprintf("cmd package install-existing %s 2>/dev/null", pkg))
			conn.Shell(fmt.Sprintf("pm enable %s 2>/dev/null", pkg))
		}
		conn.Shell("pm enable com.nautilus.nautiluslauncher/com.nautilus.nautiluslauncher.thirdparty.appmonitor.AppMonitorService 2>/dev/null")
		// Restore kiosk settings
		conn.Shell("settings put secure navigationbar_switch 0")
		conn.Shell("settings delete global force_show_navbar 2>/dev/null")
		conn.Shell("settings put secure statusbar_switch 0")
		conn.Shell("settings put secure notification_switch 0")
		conn.Shell("settings put secure ntls_launcher_preference 1")
		conn.Shell("settings put global stay_on_while_plugged_in 0")
		// Keep .bowflex file — ADB must remain enabled so user can reconnect
		conn.Shell("rm /sdcard/Download/MiscSettings/asset_manager_disabled 2>/dev/null")
		conn.Close()
		log.Success("  Stock packages and settings restored")
	}

	// --- Step 3: Remove VeloLauncher and re-enable stock launchers ---
	step(3, "Removing VeloLauncher")
	conn, err = connect()
	if err == nil {
		conn.Shell("am force-stop io.freewheel.launcher 2>/dev/null")
		out, _ := conn.Shell("pm uninstall io.freewheel.launcher 2>/dev/null")
		conn.Shell("cmd package install-existing com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("pm enable com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("pm enable com.nautilus.nautiluslauncher/.MainActivity 2>/dev/null")
		conn.Shell("pm enable com.android.launcher3 2>/dev/null")
		conn.Shell("pm enable com.android.quickstep 2>/dev/null")
		conn.Shell("cmd package set-home-activity com.nautilus.nautiluslauncher/.MainActivity 2>/dev/null")
		conn.Close()
		if strings.Contains(out, "Success") {
			log.Success("  VeloLauncher uninstalled, stock launchers re-enabled")
		} else {
			log.Dim("  VeloLauncher not installed")
		}
	}

	// --- Step 4: Remove SerialBridge + re-enable NautilusLauncher ---
	step(4, "Removing SerialBridge, restoring NautilusLauncher")
	conn, err = connect()
	if err == nil {
		conn.Shell("am force-stop io.freewheel.bridge 2>/dev/null")
		out, _ := conn.Shell("pm uninstall io.freewheel.bridge 2>/dev/null")
		conn.Shell("cmd package install-existing com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("pm enable com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Close()
		if strings.Contains(out, "Success") {
			log.Success("  SerialBridge uninstalled, NautilusLauncher re-enabled")
		} else {
			log.Dim("  SerialBridge not installed")
		}
	}

	// --- Step 6: Clean factory_reset directory ---
	step(5, "Cleaning factory_reset directory")
	conn, err = connect()
	if err == nil {
		conn.Shell("rm /mnt/sw_release/factory_reset/serialbridge.apk 2>/dev/null")
		out, _ := conn.Shell("ls /mnt/sw_release/factory_reset/*.apk 2>/dev/null")
		conn.Close()
		apkCount := 0
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if strings.HasSuffix(strings.TrimSpace(line), ".apk") {
				apkCount++
			}
		}
		if apkCount == 1 {
			log.Success("  factory_reset dir cleaned (1 APK remains: OTAClient)")
		} else if apkCount == 0 {
			log.Warn("  factory_reset dir has no APKs -- OTAClient may be missing")
		} else {
			log.Warn(fmt.Sprintf("  factory_reset dir has %d APKs -- should be exactly 1", apkCount))
		}
	}

	// --- Step 7: Finalize stock configuration ---
	step(6, "Finalizing stock configuration")
	conn, err = connect()
	if err == nil {
		conn.Shell("pm disable-user --user 0 com.google.android.gms 2>/dev/null")
		conn.Shell("pm disable-user --user 0 com.google.android.gsf 2>/dev/null")
		conn.Shell("pm disable-user --user 0 com.android.chrome 2>/dev/null")
		conn.Shell("pm enable com.android.vending 2>/dev/null")
		conn.Shell("pm enable com.google.android.webview 2>/dev/null")
		conn.Shell("pm enable com.redbend.vdmc 2>/dev/null; pm enable com.redbend.client 2>/dev/null")
		conn.Shell("pm enable com.redbend.dualpart.service.app 2>/dev/null")
		conn.Shell("settings put global software_update 1 2>/dev/null")
		conn.Shell("settings delete global auto_update 2>/dev/null")
		conn.Close()
		log.Success("  OTA re-enabled, Google apps disabled, WebView enabled")
	}

	// --- Step 8: Restart Nautilus apps ---
	step(7, "Restarting Nautilus apps")
	conn, err = connect()
	if err == nil {
		conn.Shell("cmd package install-existing com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("pm enable com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("cmd package install-existing com.nautilus.bowflex.usb 2>/dev/null")
		conn.Shell("pm enable com.nautilus.bowflex.usb 2>/dev/null")
		conn.Shell("am start -n com.nautilus.nautiluslauncher/.MainActivity")
		conn.Close()
	}
	time.Sleep(2 * time.Second)
	conn, err = connect()
	if err == nil {
		conn.Shell("am start -n com.nautilus.bowflex.usb/com.nautilus.bowflex.usb.ui.activity.splash.SplashActivity")
		conn.Close()
		log.Success("  NautilusLauncher and JRNY restarted")
	}

	// Final summary
	log.Step("")
	log.Success("=== STOCK RESTORED ===")
	log.Info("")
	log.Dim("  - SerialBridge and VeloLauncher uninstalled")
	log.Dim("  - NautilusLauncher re-enabled (stock serial handler)")
	log.Dim("  - Launcher3 re-enabled")
	log.Dim("  - JRNY, OTA, Asset Manager, AppMonitor re-enabled")
	log.Dim("  - Kiosk mode restored (navbar/statusbar hidden)")
	log.Dim("  - Nautilus apps restarted")
	log.Info("")
	log.Warn("Note: ADB remains enabled (needed to connect). Disable manually if desired.")
}
