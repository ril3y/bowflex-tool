package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed jailbreak.apk
var embeddedJailbreak []byte

//go:embed freewheelservice.apk
var embeddedFreeWheelService []byte

//go:embed velolauncher.apk
var embeddedVeloLauncher []byte

//go:embed freeride.apk
var embeddedFreeRide []byte

//go:embed logo.png
var embeddedLogo []byte

var (
	colorSuccess = color.NRGBA{R: 0, G: 255, B: 136, A: 255}
	colorError   = color.NRGBA{R: 255, G: 69, B: 96, A: 255}
	colorWarn    = color.NRGBA{R: 255, G: 200, B: 0, A: 255}
	colorInfo    = color.NRGBA{R: 136, G: 136, B: 200, A: 255}
	colorStep    = color.NRGBA{R: 100, G: 180, B: 255, A: 255}
	colorDim     = color.NRGBA{R: 100, G: 100, B: 130, A: 255}
)

type logEntry struct {
	text  string
	color color.Color
}

func main() {
	os.Setenv("FYNE_THEME", "dark")

	a := app.NewWithID("com.battlewithbytes.freewheel")
	a.Settings().SetTheme(theme.DarkTheme())
	w := a.NewWindow("FreeWheel")
	w.Resize(fyne.NewSize(520, 600))
	w.SetFixedSize(true)

	// --- Colored log (virtualized list to prevent layout growth) ---
	var logEntries []logEntry
	var logMu sync.Mutex

	logList := widget.NewList(
		func() int {
			logMu.Lock()
			defer logMu.Unlock()
			return len(logEntries)
		},
		func() fyne.CanvasObject {
			t := canvas.NewText("", color.White)
			t.TextSize = 12
			t.TextStyle = fyne.TextStyle{Monospace: true}
			return t
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			logMu.Lock()
			defer logMu.Unlock()
			t := obj.(*canvas.Text)
			if id < len(logEntries) {
				t.Text = logEntries[id].text
				t.Color = logEntries[id].color
				t.Refresh()
			}
		},
	)
	appendLog := func(clr color.Color, msg string) {
		logMu.Lock()
		logEntries = append(logEntries, logEntry{text: msg, color: clr})
		logMu.Unlock()
		fyne.Do(func() {
			logList.Refresh()
			logList.ScrollToBottom()
		})
	}

	logInfo := func(msg string) { appendLog(colorInfo, msg) }
	logSuccess := func(msg string) { appendLog(colorSuccess, msg) }
	logError := func(msg string) { appendLog(colorError, msg) }
	logWarn := func(msg string) { appendLog(colorWarn, msg) }
	logStep := func(msg string) { appendLog(colorStep, msg) }
	logDim := func(msg string) { appendLog(colorDim, msg) }

	// --- Small icon-only log controls ---
	copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		logMu.Lock()
		defer logMu.Unlock()
		var sb strings.Builder
		for _, e := range logEntries {
			sb.WriteString(e.text)
			sb.WriteString("\n")
		}
		w.Clipboard().SetContent(sb.String())
	})

	clearBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		logMu.Lock()
		logEntries = nil
		logMu.Unlock()
		fyne.Do(func() {
			logList.Refresh()
		})
	})

	// --- Device list ---
	var foundDevices []string
	selectedDevice := ""
	deviceList := widget.NewList(
		func() int { return len(foundDevices) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < len(foundDevices) {
				obj.(*widget.Label).SetText(fmt.Sprintf("  %s  (port 5555)", foundDevices[id]))
			}
		},
	)
	deviceList.OnSelected = func(id widget.ListItemID) {
		if id < len(foundDevices) {
			selectedDevice = foundDevices[id]
		}
	}

	// --- Manual IP entry ---
	ipEntry := widget.NewEntry()
	ipEntry.SetPlaceHolder("192.168.1.xxx")

	// --- Buttons ---
	var scanBtn, jailbreakBtn, restoreBtn, factoryResetBtn *widget.Button
	_ = restoreBtn      // used in closures
	_ = factoryResetBtn // used in closures

	scanBtn = widget.NewButtonWithIcon("Scan Network", theme.SearchIcon(), func() {
		scanBtn.Disable()
		logInfo("Scanning network for Bowflex bikes (port 5555)...")
		go func() {
			devices := scanNetwork(logInfo)
			foundDevices = devices
			deviceList.Refresh()
			if len(devices) > 0 {
				deviceList.Select(0)
				// Check if first device is already jailbroken
				logDim("Checking device status...")
				conn, err := adbConnect(devices[0], 5555, logWarn)
				if err == nil {
					out, _ := conn.Shell("pm list packages 2>/dev/null | grep freewheel.service")
					conn.Close()
					if strings.Contains(out, "freewheel.service") {
						restoreBtn.Show()
						factoryResetBtn.Show()
						logInfo("Device is jailbroken -- Restore Stock / Factory Reset available")
					}
				}
			}
			scanBtn.Enable()
			if len(devices) == 0 {
				logWarn("No devices found. Enter IP manually or check instructions above.")
			}
		}()
	})

	jailbreakBtn = widget.NewButtonWithIcon("  Jailbreak!  ", theme.ConfirmIcon(), func() {
		ip := ipEntry.Text
		if ip == "" {
			ip = selectedDevice
		}
		if ip == "" {
			logError("No device selected and no IP entered!")
			return
		}
		if strings.Contains(ip, ":") {
			ip = strings.Split(ip, ":")[0]
		}
		jailbreakBtn.Disable()
		scanBtn.Disable()
		go func() {
			runJailbreak(ip, logStep, logInfo, logSuccess, logError, logWarn, logDim)
			jailbreakBtn.Enable()
			scanBtn.Enable()
			restoreBtn.Show()
			factoryResetBtn.Show()
		}()
	})
	jailbreakBtn.Importance = widget.HighImportance

	restoreBtn = widget.NewButtonWithIcon("Restore Stock", theme.MediaReplayIcon(), func() {
		ip := ipEntry.Text
		if ip == "" {
			ip = selectedDevice
		}
		if ip == "" {
			logError("No device selected and no IP entered!")
			return
		}
		if strings.Contains(ip, ":") {
			ip = strings.Split(ip, ":")[0]
		}
		restoreBtn.Disable()
		jailbreakBtn.Disable()
		scanBtn.Disable()
		go func() {
			runRestore(ip, logStep, logInfo, logSuccess, logError, logWarn, logDim)
			restoreBtn.Hide()
			restoreBtn.Enable()
			jailbreakBtn.Enable()
			scanBtn.Enable()
		}()
	})
	restoreBtn.Importance = widget.DangerImportance
	restoreBtn.Hide()

	factoryResetBtn = widget.NewButtonWithIcon("Factory Reset", theme.WarningIcon(), func() {
		ip := ipEntry.Text
		if ip == "" {
			ip = selectedDevice
		}
		if ip == "" {
			logError("No device selected and no IP entered!")
			return
		}
		if strings.Contains(ip, ":") {
			ip = strings.Split(ip, ":")[0]
		}
		factoryResetBtn.Disable()
		restoreBtn.Disable()
		jailbreakBtn.Disable()
		scanBtn.Disable()
		go func() {
			runFactoryReset(ip, logStep, logInfo, logSuccess, logError, logWarn, logDim)
			factoryResetBtn.Hide()
			restoreBtn.Hide()
			factoryResetBtn.Enable()
			restoreBtn.Enable()
			jailbreakBtn.Enable()
			scanBtn.Enable()
		}()
	})
	factoryResetBtn.Importance = widget.DangerImportance
	factoryResetBtn.Hide()

	// --- Logo ---
	logoRes := fyne.NewStaticResource("logo.png", embeddedLogo)
	logoImg := canvas.NewImageFromResource(logoRes)
	logoImg.FillMode = canvas.ImageFillContain
	logoImg.SetMinSize(fyne.NewSize(36, 36))

	// Set window icon
	w.SetIcon(logoRes)

	// --- Header: logo + title + subtitle in one compact row ---
	titleText := canvas.NewText("FreeWheel", colorSuccess)
	titleText.TextSize = 22
	titleText.TextStyle = fyne.TextStyle{Bold: true}

	subtitleText := canvas.NewText("Bowflex VeloCore Jailbreak", color.NRGBA{R: 140, G: 140, B: 160, A: 255})
	subtitleText.TextSize = 11

	titleBlock := container.NewVBox(titleText, subtitleText)
	headerRow := container.NewHBox(logoImg, container.NewPadded(titleBlock))

	// --- Quick-start hints (minimal, replaces wall of text) ---
	hint1 := canvas.NewText("1. On bike: tap top-right corner 9x, open Utility App", color.NRGBA{R: 170, G: 170, B: 190, A: 255})
	hint1.TextSize = 11
	hint2 := canvas.NewText("2. Here: Scan or enter IP, then hit Jailbreak", color.NRGBA{R: 170, G: 170, B: 190, A: 255})
	hint2.TextSize = 11
	hint3 := canvas.NewText("3. On bike: tap Allow if USB debugging prompt appears", color.NRGBA{R: 170, G: 170, B: 190, A: 255})
	hint3.TextSize = 11

	hintsBox := container.NewVBox(hint1, hint2, hint3)

	// --- Accent line under header ---
	accentLine := canvas.NewRectangle(color.NRGBA{R: 0, G: 255, B: 136, A: 60})
	accentLine.SetMinSize(fyne.NewSize(0, 2))

	// --- Device section: compact card-style ---
	deviceListSized := container.NewGridWrap(fyne.NewSize(456, 64), deviceList)

	ipRow := container.NewBorder(nil, nil,
		widget.NewLabel("IP:"), scanBtn,
		ipEntry,
	)

	deviceSection := container.NewVBox(
		deviceListSized,
		ipRow,
	)
	deviceCard := widget.NewCard("", "Target Device", deviceSection)

	// --- Action buttons: prominent, well-spaced ---
	buttonRow := container.NewHBox(
		layout.NewSpacer(),
		factoryResetBtn,
		restoreBtn,
		jailbreakBtn,
	)

	// --- Top panel (everything above log) ---
	topPanel := container.NewVBox(
		container.NewPadded(headerRow),
		accentLine,
		container.NewPadded(hintsBox),
		container.NewPadded(deviceCard),
		container.NewPadded(buttonRow),
	)

	// --- Log header with inline controls ---
	logTitle := canvas.NewText("Output", color.NRGBA{R: 180, G: 180, B: 200, A: 255})
	logTitle.TextSize = 12
	logTitle.TextStyle = fyne.TextStyle{Bold: true}

	logToolbar := container.NewHBox(
		logTitle,
		layout.NewSpacer(),
		copyBtn,
		clearBtn,
	)

	// --- Log panel with dark background ---
	logBg := canvas.NewRectangle(color.NRGBA{R: 18, G: 18, B: 24, A: 255})
	logArea := container.NewStack(logBg, logList)

	logPanel := container.NewBorder(logToolbar, nil, nil, nil, logArea)

	// --- Main layout: top is fixed, log expands to fill remaining space ---
	w.SetContent(container.NewPadded(
		container.NewBorder(topPanel, nil, nil, nil, logPanel),
	))

	// Startup
	go func() {
		time.Sleep(300 * time.Millisecond)
		logInfo("FreeWheel ready. Native ADB -- no external tools needed.")
	}()

	w.ShowAndRun()
}

// queryServiceAPI makes an HTTP request to FreeWheelService on the bike.
func queryServiceAPI(ip, method, path string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s:8888%s", ip, path)
	var resp *http.Response
	var err error

	client := &http.Client{Timeout: 5 * time.Second}
	if method == "POST" {
		resp, err = client.Post(url, "application/json", nil)
	} else {
		resp, err = client.Get(url)
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	return result, err
}

func runJailbreak(ip string, logStep, logInfo, logSuccess, logError, logWarn, logDim func(string)) {
	totalSteps := 12

	step := func(n int, name string) {
		logStep(fmt.Sprintf("[STEP %d/%d] %s", n, totalSteps, name))
	}

	// Helper: fresh connection (ADB drops after a few shell commands)
	connect := func() (*AdbConn, error) {
		return adbConnect(ip, 5555, logWarn)
	}

	// Helper: install APK by name
	installAPK := func(name, pkg string) bool {
		apkPath := findAPK(name)
		if apkPath == "" {
			logError(fmt.Sprintf("  %s.apk not found!", name))
			return false
		}
		logDim(fmt.Sprintf("  Using: %s", apkPath))

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
			logError(fmt.Sprintf("  Connection failed: %v", err))
			return false
		}
		tmpPath := fmt.Sprintf("/data/local/tmp/_%s.apk", name)
		err = conn.Push(apkPath, tmpPath, 0644)
		conn.Close()
		if err != nil {
			logError(fmt.Sprintf("  Push failed: %v", err))
			return false
		}

		// Install
		conn, err = connect()
		if err != nil {
			logError(fmt.Sprintf("  Connection failed: %v", err))
			return false
		}
		out, _ := conn.Shell(fmt.Sprintf("pm install -r %s", tmpPath))
		conn.Close()
		if !strings.Contains(out, "Success") {
			logError(fmt.Sprintf("  Install failed: %s", strings.TrimSpace(out)))
			return false
		}

		// Cleanup
		conn, err = connect()
		if err == nil {
			conn.Shell(fmt.Sprintf("rm %s", tmpPath))
			conn.Close()
		}
		return true
	}

	// --- Step 1: Connect ---
	step(1, "Connecting to bike")
	logDim(fmt.Sprintf("  Target: %s:5555", ip))

	conn, err := connect()
	if err != nil {
		logError(fmt.Sprintf("  Connection failed: %v", err))
		logWarn("  Make sure: 1) Bike is on same WiFi  2) Utility App was opened (9-tap)")
		return
	}
	logSuccess("  Connected!")

	// --- Step 2: Verify ---
	step(2, "Verifying ADB shell")
	out, err := conn.Shell("id")
	if err != nil {
		logError(fmt.Sprintf("  Shell failed: %v", err))
		conn.Close()
		return
	}
	idStr := strings.TrimSpace(out)
	if idx := strings.Index(idStr, " groups="); idx > 0 {
		idStr = idStr[:idx]
	}
	logSuccess(fmt.Sprintf("  %s", idStr))

	// --- Step 3: Stop AppMonitor (immediate) ---
	step(3, "Stopping AppMonitor watchdog")
	conn.Shell("am stop-service com.nautilus.nautiluslauncher/.thirdparty.appmonitor.AppMonitorService 2>/dev/null")
	logSuccess("  AppMonitor service stopped")

	// --- Step 4: Check current state ---
	step(4, "Checking current state")
	out, _ = conn.Shell("pm list packages 2>/dev/null | grep -iE 'freewheel.service|bowflex.jailbreak|freewheel.launcher|freewheel.freeride|bowflex.serialbridge'")
	conn.Close()

	if strings.Contains(out, "freewheel.service") {
		logDim("  FreeWheelService present -- will reinstall")
	}
	if strings.Contains(out, "bowflex.jailbreak") {
		logDim("  Jailbreak APK present -- will reinstall")
	}
	if strings.Contains(out, "freewheel.launcher") {
		logDim("  VeloLauncher present -- will reinstall")
	}
	if strings.Contains(out, "freewheel.freeride") {
		logDim("  FreeRide present -- will reinstall")
	}
	if strings.Contains(out, "bowflex.serialbridge") {
		logWarn("  Old SerialBridge found -- will remove (replaced by FreeWheelService)")
	}

	// --- Step 5: Install FreeWheelService APK (platform-signed, does heavy lifting) ---
	step(5, "Installing FreeWheelService APK")
	if !installAPK("freewheelservice", "com.freewheel.service") {
		return
	}
	logSuccess("  FreeWheelService installed (platform-signed, uid 1000)")

	// --- Step 6: Start FreeWheelService (it applies all jailbreak settings) ---
	step(6, "Starting FreeWheelService")
	conn, err = connect()
	if err != nil {
		logError(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	conn.Shell("am start-foreground-service -n com.freewheel.service/.FreeWheelService")
	conn.Close()
	logDim("  Service started -- applying jailbreak settings...")

	// --- Step 7: Wait for service to apply settings (poll /status API) ---
	step(7, "Waiting for FreeWheelService to apply settings")
	var apiOk bool
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		status, err := queryServiceAPI(ip, "GET", "/status")
		if err == nil {
			if jb, ok := status["jailbroken"].(bool); ok && jb {
				apiOk = true
				logSuccess("  FreeWheelService API responding -- jailbreak applied!")
				// Show package states
				if pkgs, ok := status["packages"].(map[string]interface{}); ok {
					for pkg, state := range pkgs {
						logDim(fmt.Sprintf("    %s: %v", pkg, state))
					}
				}
				break
			}
		}
		logDim(fmt.Sprintf("  Waiting... (%d/10)", i+1))
	}
	if !apiOk {
		logWarn("  API not responding -- checking via ADB")
		conn, err = connect()
		if err == nil {
			// Verify JRNY is disabled
			out, _ = conn.Shell("pm list packages -d 2>/dev/null | grep bowflex.usb")
			if strings.Contains(out, "bowflex.usb") {
				logSuccess("  JRNY confirmed disabled via ADB")
			} else {
				logWarn("  JRNY may not be disabled -- service may need reboot")
			}
			conn.Close()
		}
	}

	// --- Step 8: Install VeloLauncher ---
	step(8, "Installing VeloLauncher")
	vlApk := findAPK("velolauncher")
	if vlApk == "" {
		logWarn("  velolauncher.apk not found -- skipping")
	} else {
		if installAPK("velolauncher", "io.freewheel.launcher") {
			logSuccess("  VeloLauncher installed")
			conn, err = connect()
			if err == nil {
				conn.Shell("cmd package set-home-activity io.freewheel.launcher/.MainActivity")
				conn.Shell("pm disable-user --user 0 com.android.launcher3 2>/dev/null")
				conn.Close()
				logSuccess("  Set as default home, Launcher3 disabled")
			}
		}
	}

	// --- Step 9: Install FreeRide ---
	step(9, "Installing FreeRide APK")
	frApk := findAPK("freeride")
	if frApk == "" {
		logWarn("  freeride.apk not found -- skipping")
	} else {
		if installAPK("freeride", "io.freewheel.freeride") {
			logSuccess("  FreeRide installed")
			conn, err = connect()
			if err == nil {
				conn.Shell("appops set io.freewheel.freeride SYSTEM_ALERT_WINDOW allow")
				conn.Shell("pm grant io.freewheel.freeride android.permission.WRITE_SECURE_SETTINGS")
				conn.Close()
				logDim("  Overlay + settings permissions granted")
			}
		}
	}

	// --- Step 10: Install Jailbreak APK (overlay/breakout utility) ---
	step(10, "Installing jailbreak APK")
	if installAPK("jailbreak", "com.bowflex.jailbreak") {
		logSuccess("  Jailbreak APK installed")
		conn, err = connect()
		if err == nil {
			conn.Shell("appops set com.bowflex.jailbreak SYSTEM_ALERT_WINDOW allow")
			conn.Close()
		}
	}

	// --- Step 11: Verify Google apps disabled (2GB RAM) ---
	step(11, "Verifying Google apps disabled")
	conn, err = connect()
	if err == nil {
		googleApps := []struct{ pkg, name string }{
			{"com.google.android.gms", "Play Services"},
			{"com.google.android.gsf", "Google Services Framework"},
			{"com.android.vending", "Play Store"},
			{"com.android.chrome", "Chrome"},
		}
		for _, a := range googleApps {
			conn.Shell(fmt.Sprintf("pm disable-user --user 0 %s 2>/dev/null", a.pkg))
			logDim(fmt.Sprintf("  Disabled: %s (saves ~100MB RAM)", a.name))
		}
		r, _ := conn.Shell("pm enable com.google.android.webview 2>/dev/null")
		if strings.Contains(r, "new state: enabled") {
			logSuccess("  Enabled: WebView")
		}

		// Remove old SerialBridge if present
		out, _ = conn.Shell("pm list packages 2>/dev/null | grep bowflex.serialbridge")
		if strings.Contains(out, "bowflex.serialbridge") {
			conn.Shell("am force-stop com.bowflex.serialbridge 2>/dev/null")
			conn.Shell("pm uninstall com.bowflex.serialbridge 2>/dev/null")
			logDim("  Removed old SerialBridge (replaced by FreeWheelService)")
		}
		conn.Close()
		logSuccess("  Google apps kept disabled (device has only 2GB RAM)")
	}

	// --- Step 12: Go home + final status check ---
	step(12, "Final setup")

	// Start overlay service
	conn, err = connect()
	if err == nil {
		conn.Shell("am start -n com.bowflex.jailbreak/.MainActivity 2>/dev/null")
		conn.Close()
	}
	time.Sleep(2 * time.Second)
	conn, err = connect()
	if err == nil {
		conn.Shell("am startservice -n com.bowflex.jailbreak/.OverlayService 2>/dev/null")
		conn.Shell("input keyevent KEYCODE_HOME")
		conn.Close()
		logSuccess("  Home screen active with overlay")
	}

	// Final API status check
	status, err := queryServiceAPI(ip, "GET", "/status")
	if err == nil {
		logDim(fmt.Sprintf("  API: %v", status))
	}

	// Check UCB/serial port
	ucb, err := queryServiceAPI(ip, "GET", "/ucb")
	if err == nil {
		if alive, ok := ucb["tcp_9999"].(bool); ok && alive {
			logSuccess("  TCP:9999 alive (NautilusLauncher serving serial)")
		} else {
			logDim("  TCP:9999 not yet listening (NautilusLauncher may need a moment)")
		}
	}

	// Final summary
	logStep("")
	logSuccess("=== JAILBREAK COMPLETE ===")
	logInfo("")
	logDim("  Architecture: Option B (NautilusLauncher handles serial/UCB)")
	logDim("  FreeWheelService manages: JRNY disabled, AppMonitor killed, OTA blocked")
	logDim("  NautilusLauncher still running: serial port, TCP:9999, UCB protocol")
	logDim("  VeloLauncher set as home screen (free, no subscription)")
	logDim("  FreeRide fitness app installed with overlay permission")
	logDim("  Google apps kept disabled (2GB RAM)")
	logDim("  ADB on port 5555, navbar enabled, kiosk mode off")
	logDim("  Screen stays on while plugged in, WiFi never sleeps")
	logDim("  Status API: http://" + ip + ":8888/status")
	logInfo("")
	logInfo("All services persist across reboots via FreeWheelService BootReceiver.")
}

func scanNetwork(logInfo func(string)) []string {
	var results []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	subnets := getLocalSubnets()
	if len(subnets) == 0 {
		logInfo("Could not determine local network. Trying 192.168.1.0/24")
		subnets = []string{"192.168.1"}
	}

	for _, subnet := range subnets {
		logInfo(fmt.Sprintf("Scanning %s.0/24...", subnet))
		for i := 1; i < 255; i++ {
			wg.Add(1)
			ip := fmt.Sprintf("%s.%d", subnet, i)
			go func(ip string) {
				defer wg.Done()
				c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:5555", ip), 500*time.Millisecond)
				if err == nil {
					c.Close()
					mu.Lock()
					results = append(results, ip)
					mu.Unlock()
					logInfo(fmt.Sprintf("Found device: %s", ip))
				}
			}(ip)
		}
	}
	wg.Wait()
	return results
}

func getLocalSubnets() []string {
	var subnets []string
	seen := map[string]bool{}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			subnet := fmt.Sprintf("%d.%d.%d", ip[0], ip[1], ip[2])
			if !seen[subnet] {
				seen[subnet] = true
				subnets = append(subnets, subnet)
			}
		}
	}
	return subnets
}

func runRestore(ip string, logStep, logInfo, logSuccess, logError, logWarn, logDim func(string)) {
	totalSteps := 10

	step := func(n int, name string) {
		logStep(fmt.Sprintf("[STEP %d/%d] %s", n, totalSteps, name))
	}

	connect := func() (*AdbConn, error) {
		return adbConnect(ip, 5555, logWarn)
	}

	// --- Step 1: Connect ---
	step(1, "Connecting to bike")
	conn, err := connect()
	if err != nil {
		logError(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	logSuccess("  Connected!")
	conn.Close()

	// --- Step 2: Try API restore first ---
	step(2, "Restoring stock via FreeWheelService API")
	apiResult, err := queryServiceAPI(ip, "POST", "/restore-stock")
	if err == nil && apiResult != nil {
		logSuccess("  FreeWheelService restored stock settings via API")
	} else {
		logDim("  API not available -- falling back to shell commands")
		conn, err = connect()
		if err == nil {
			// Manually re-enable stock packages
			stockPkgs := []string{
				"com.nautilus.bowflex.usb",
				"com.redbend.client", "com.redbend.vdmc", "com.redbend.dualpart.service.app",
				"com.nautilus.g4assetmanager", "com.nautilus.nlssbcsystemsettings",
			}
			for _, pkg := range stockPkgs {
				conn.Shell(fmt.Sprintf("pm enable %s 2>/dev/null", pkg))
			}
			// Re-enable AppMonitor component
			conn.Shell("pm enable com.nautilus.nautiluslauncher/com.nautilus.nautiluslauncher.thirdparty.appmonitor.AppMonitorService 2>/dev/null")
			// Restore kiosk settings
			conn.Shell("settings put secure navigationbar_switch 0")
			conn.Shell("settings delete global force_show_navbar 2>/dev/null")
			conn.Shell("settings put secure statusbar_switch 0")
			conn.Shell("settings put secure notification_switch 0")
			conn.Shell("settings put secure ntls_launcher_preference 1")
			conn.Shell("settings put global stay_on_while_plugged_in 0")
			conn.Shell("rm /sdcard/Music/.bowflex 2>/dev/null")
			conn.Shell("rm /sdcard/Download/MiscSettings/asset_manager_disabled 2>/dev/null")
			conn.Close()
			logSuccess("  Stock packages and settings restored via ADB")
		}
	}

	// --- Step 3: Stop and uninstall FreeWheelService ---
	step(3, "Removing FreeWheelService")
	conn, err = connect()
	if err == nil {
		conn.Shell("am force-stop com.freewheel.service 2>/dev/null")
		out, _ := conn.Shell("pm uninstall com.freewheel.service 2>/dev/null")
		conn.Close()
		if strings.Contains(out, "Success") {
			logSuccess("  FreeWheelService uninstalled")
		} else {
			logDim("  FreeWheelService not installed or already removed")
		}
	}

	// --- Step 4: Stop and uninstall jailbreak APK ---
	step(4, "Removing jailbreak APK")
	conn, err = connect()
	if err == nil {
		conn.Shell("am stopservice -n com.bowflex.jailbreak/.OverlayService 2>/dev/null")
		conn.Shell("am force-stop com.bowflex.jailbreak 2>/dev/null")
		out, _ := conn.Shell("pm uninstall com.bowflex.jailbreak 2>/dev/null")
		conn.Close()
		if strings.Contains(out, "Success") {
			logSuccess("  Jailbreak APK uninstalled")
		} else {
			logDim("  Jailbreak APK not installed or already removed")
		}
	}

	// --- Step 5: Remove VeloLauncher and re-enable Launcher3 ---
	step(5, "Removing VeloLauncher")
	conn, err = connect()
	if err == nil {
		conn.Shell("am force-stop io.freewheel.launcher 2>/dev/null")
		out, _ := conn.Shell("pm uninstall io.freewheel.launcher 2>/dev/null")
		conn.Shell("pm enable com.android.launcher3 2>/dev/null")
		conn.Close()
		if strings.Contains(out, "Success") {
			logSuccess("  VeloLauncher uninstalled, Launcher3 re-enabled")
		} else {
			logDim("  VeloLauncher not installed")
		}
	}

	// --- Step 6: Remove FreeRide ---
	step(6, "Removing FreeRide")
	conn, err = connect()
	if err == nil {
		conn.Shell("am force-stop io.freewheel.freeride 2>/dev/null")
		out, _ := conn.Shell("pm uninstall io.freewheel.freeride 2>/dev/null")
		conn.Close()
		if strings.Contains(out, "Success") {
			logSuccess("  FreeRide uninstalled")
		} else {
			logDim("  FreeRide not installed")
		}
	}

	// --- Step 7: Remove old SerialBridge if present ---
	step(7, "Removing old SerialBridge (if present)")
	conn, err = connect()
	if err == nil {
		conn.Shell("am force-stop com.bowflex.serialbridge 2>/dev/null")
		out, _ := conn.Shell("pm uninstall com.bowflex.serialbridge 2>/dev/null")
		conn.Close()
		if strings.Contains(out, "Success") {
			logSuccess("  Old SerialBridge uninstalled")
		} else {
			logDim("  SerialBridge not installed")
		}
	}

	// --- Step 8: Clean factory_reset directory ---
	step(8, "Cleaning factory_reset directory")
	conn, err = connect()
	if err == nil {
		conn.Shell("rm /mnt/sw_release/factory_reset/jailbreak.apk 2>/dev/null")
		conn.Shell("rm /mnt/sw_release/factory_reset/serialbridge.apk 2>/dev/null")
		conn.Shell("rm /mnt/sw_release/factory_reset/freewheelservice.apk 2>/dev/null")
		out, _ := conn.Shell("ls /mnt/sw_release/factory_reset/*.apk 2>/dev/null")
		conn.Close()
		apkCount := 0
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if strings.HasSuffix(strings.TrimSpace(line), ".apk") {
				apkCount++
			}
		}
		if apkCount == 1 {
			logSuccess("  factory_reset dir cleaned (1 APK remains: OTAClient)")
		} else if apkCount == 0 {
			logWarn("  factory_reset dir has no APKs -- OTAClient may be missing")
		} else {
			logWarn(fmt.Sprintf("  factory_reset dir has %d APKs -- should be exactly 1", apkCount))
		}
	}

	// --- Step 9: Re-disable Google apps and re-enable OTA ---
	step(9, "Finalizing stock configuration")
	conn, err = connect()
	if err == nil {
		// Re-disable Google apps (stock state)
		conn.Shell("pm disable-user --user 0 com.android.vending 2>/dev/null")
		conn.Shell("pm disable-user --user 0 com.android.chrome 2>/dev/null")
		conn.Shell("pm disable-user --user 0 com.google.android.webview 2>/dev/null")
		// Re-enable OTA
		conn.Shell("pm enable com.redbend.vdmc 2>/dev/null; pm enable com.redbend.client 2>/dev/null")
		conn.Shell("pm enable com.redbend.dualpart.service.app 2>/dev/null")
		conn.Shell("settings put global software_update 1 2>/dev/null")
		conn.Shell("settings delete global auto_update 2>/dev/null")
		conn.Close()
		logSuccess("  OTA re-enabled, Google apps re-disabled (stock state)")
	}

	// --- Step 10: Restart Nautilus apps ---
	step(10, "Restarting Nautilus apps")
	conn, err = connect()
	if err == nil {
		conn.Shell("pm enable com.nautilus.nautiluslauncher 2>/dev/null")
		conn.Shell("pm enable com.nautilus.bowflex.usb 2>/dev/null")
		conn.Shell("am start -n com.nautilus.nautiluslauncher/.MainActivity")
		conn.Close()
	}
	time.Sleep(2 * time.Second)
	conn, err = connect()
	if err == nil {
		conn.Shell("am start -n com.nautilus.bowflex.usb/com.nautilus.bowflex.usb.ui.activity.splash.SplashActivity")
		conn.Close()
		logSuccess("  NautilusLauncher and JRNY restarted")
	}

	// Final summary
	logStep("")
	logSuccess("=== STOCK RESTORED ===")
	logInfo("")
	logDim("  - FreeWheelService, Jailbreak, VeloLauncher, FreeRide uninstalled")
	logDim("  - SerialBridge removed (if present)")
	logDim("  - Launcher3 re-enabled")
	logDim("  - JRNY, OTA, Asset Manager, AppMonitor re-enabled")
	logDim("  - Kiosk mode restored (navbar/statusbar hidden)")
	logDim("  - Nautilus apps restarted")
	logInfo("")
	logWarn("Note: ADB remains enabled (needed to connect). Disable manually if desired.")
}

func runFactoryReset(ip string, logStep, logInfo, logSuccess, logError, logWarn, logDim func(string)) {
	totalSteps := 5

	step := func(n int, name string) {
		logStep(fmt.Sprintf("[STEP %d/%d] %s", n, totalSteps, name))
	}

	connect := func() (*AdbConn, error) {
		return adbConnect(ip, 5555, logWarn)
	}

	logWarn("=== FACTORY RESET ===")
	logWarn("This will ERASE ALL DATA on the bike and restore to stock.")
	logWarn("The bike will reboot and return to the JRNY setup screen.")
	logInfo("")

	// --- Step 1: Connect ---
	step(1, "Connecting to bike")
	conn, err := connect()
	if err != nil {
		logError(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	logSuccess("  Connected!")

	// --- Step 2: Clean factory_reset directory ---
	step(2, "Cleaning factory_reset directory")
	conn.Shell("rm /mnt/sw_release/factory_reset/jailbreak.apk 2>/dev/null")
	conn.Shell("rm /mnt/sw_release/factory_reset/serialbridge.apk 2>/dev/null")
	conn.Shell("rm /mnt/sw_release/factory_reset/freewheelservice.apk 2>/dev/null")
	out, _ := conn.Shell("ls /mnt/sw_release/factory_reset/*.apk 2>/dev/null")
	conn.Close()
	apkCount := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), ".apk") {
			apkCount++
		}
	}
	if apkCount == 1 {
		logSuccess("  factory_reset dir cleaned (OTAClient only)")
	} else {
		logWarn(fmt.Sprintf("  factory_reset dir has %d APKs (expected 1)", apkCount))
	}

	// --- Step 3: Re-enable all stock packages ---
	step(3, "Re-enabling stock packages")
	conn, err = connect()
	if err != nil {
		logError(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	stockPkgs := []string{
		"com.nautilus.nautiluslauncher",
		"com.android.launcher3",
		"com.redbend.vdmc",
		"com.redbend.client",
		"com.redbend.dualpart.service.app",
		"com.nautilus.g4assetmanager",
		"com.nautilus.nlssbcsystemsettings",
		"com.nautilus.bowflex.usb",
		"com.android.vending",
		"com.android.chrome",
		"com.google.android.webview",
		"com.google.android.gms",
		"com.google.android.gsf",
	}
	for _, pkg := range stockPkgs {
		conn.Shell(fmt.Sprintf("pm enable %s 2>/dev/null", pkg))
	}
	conn.Close()
	logSuccess("  All stock packages re-enabled")

	// --- Step 4: Restore kiosk settings ---
	step(4, "Restoring kiosk settings")
	conn, err = connect()
	if err != nil {
		logError(fmt.Sprintf("  Connection failed: %v", err))
		return
	}
	conn.Shell("settings put secure navigationbar_switch 0")
	conn.Shell("settings delete global force_show_navbar 2>/dev/null")
	conn.Shell("settings put secure statusbar_switch 0")
	conn.Shell("settings put system screen_off_timeout 2147483647")
	conn.Shell("settings put global software_update 1 2>/dev/null")
	conn.Shell("settings delete global auto_update 2>/dev/null")
	conn.Shell("rm /sdcard/Download/MiscSettings/asset_manager_disabled 2>/dev/null")
	conn.Shell("rm /sdcard/Music/.bowflex 2>/dev/null")
	conn.Close()
	logSuccess("  Kiosk settings restored")

	// --- Step 5: Trigger Android factory reset ---
	step(5, "Triggering factory reset")
	logWarn("  The bike will reboot and erase all user data.")
	logWarn("  ADB will disconnect. This is expected.")

	// Try FreeWheelService API first
	apiResult, err := queryServiceAPI(ip, "POST", "/factory-reset")
	if err == nil && apiResult != nil {
		logSuccess("  Factory reset triggered via FreeWheelService API!")
	} else {
		// Fall back to broadcast
		conn, err = connect()
		if err != nil {
			logError(fmt.Sprintf("  Connection failed: %v", err))
			return
		}

		// Try FreeWheelService broadcast
		out, _ = conn.Shell("pm list packages 2>/dev/null | grep freewheel.service")
		hasFWS := strings.Contains(out, "freewheel.service")

		// Try old SerialBridge broadcast too
		out2, _ := conn.Shell("pm list packages 2>/dev/null | grep bowflex.serialbridge")
		hasSB := strings.Contains(out2, "bowflex.serialbridge")
		conn.Close()

		conn, err = connect()
		if err != nil {
			logError(fmt.Sprintf("  Connection failed: %v", err))
			return
		}

		if hasFWS {
			logInfo("  Using FreeWheelService (uid 1000) to trigger factory reset...")
			out, _ = conn.Shell("am broadcast -a com.freewheel.service.FACTORY_RESET -p com.freewheel.service 2>&1")
		} else if hasSB {
			logInfo("  Using SerialBridge (uid 1000) to trigger factory reset...")
			out, _ = conn.Shell("am broadcast -a com.bowflex.serialbridge.FACTORY_RESET -p com.bowflex.serialbridge 2>&1")
		} else {
			logInfo("  No system app available, trying direct broadcast...")
			out, _ = conn.Shell("am broadcast -a android.intent.action.FACTORY_RESET -p android -f 268435456 --es android.intent.extra.REASON MasterClearConfirm 2>&1")
		}
		conn.Close()

		if strings.Contains(out, "result=-1") || strings.Contains(out, "Broadcasting") {
			logSuccess("  Factory reset triggered!")
		} else {
			logError(fmt.Sprintf("  Factory reset result: %s", strings.TrimSpace(out)))
			logWarn("  The broadcast may not have triggered the reset.")
			logInfo("  Alternative: enable Settings, then navigate to System > Reset > Erase all data")
			return
		}
	}

	logInfo("")
	logSuccess("=== FACTORY RESET INITIATED ===")
	logInfo("")
	logDim("  The bike is now wiping all data and rebooting.")
	logDim("  This takes 2-3 minutes. Do not unplug the bike.")
	logDim("  When complete, JRNY will show the initial setup screen.")
	logDim("  ADB will no longer be accessible (re-enable via 9-tap + Utility App).")
}

// findAPK returns a path to the APK, extracting from embedded data if needed.
func findAPK(name string) string {
	// First check for local overrides (next to exe or cwd)
	var candidates []string
	switch name {
	case "jailbreak":
		candidates = []string{"jailbreak.apk", "bowflex-jailbreak.apk"}
	case "freewheelservice":
		candidates = []string{"freewheelservice.apk"}
	case "velolauncher":
		candidates = []string{"velolauncher.apk"}
	case "freeride":
		candidates = []string{"freeride.apk"}
	default:
		candidates = []string{name + ".apk"}
	}

	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
		if exeDir != "" {
			p := filepath.Join(exeDir, c)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// Use embedded APK — write to temp file
	var data []byte
	switch name {
	case "jailbreak":
		data = embeddedJailbreak
	case "freewheelservice":
		data = embeddedFreeWheelService
	case "velolauncher":
		data = embeddedVeloLauncher
	case "freeride":
		data = embeddedFreeRide
	}
	if len(data) > 0 {
		tmp := filepath.Join(os.TempDir(), name+".apk")
		if err := os.WriteFile(tmp, data, 0644); err == nil {
			return tmp
		}
	}

	return ""
}
