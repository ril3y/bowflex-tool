# Fitness Bike Launcher Design Guide
## Actionable Recommendations for 1920x1080 Landscape Home Screen (Jetpack Compose)

Research compiled from Peloton, NordicTrack/iFit, Android TV Leanback, Material Design 3, and fitness UX best practices.

---

## 1. Layout Architecture

### Overall Structure: Netflix-style Row Browser
All premium fitness devices (Peloton, iFit 2.0) use the same proven pattern: **vertically scrollable rows of horizontally scrollable cards**, identical to streaming platforms.

**Recommended layout (top to bottom):**

```
+------------------------------------------------------------------+
| [Nav Rail]  |  Hero / Featured Area (full-width card or pager)   |
|             |                                                     |
|  Quick      |---------------------------------------------------|
|  Ride       |  "Continue" row  (recent/in-progress workouts)     |
|             |---------------------------------------------------|
|  History    |  "Quick Start" row  (Just Ride, scenic, etc.)      |
|             |---------------------------------------------------|
|  Apps       |  "Recommended" row  (personalized suggestions)     |
|             |---------------------------------------------------|
|  Settings   |  "Entertainment" row (streaming apps)              |
|             |---------------------------------------------------|
|             |  "Stats" row (recent ride cards with metrics)       |
+------------------------------------------------------------------+
```

**Key decisions:**
- **Left navigation rail** (not bottom tabs) -- iFit 2.0 switched to this; it preserves vertical space on landscape displays and follows M3 large-screen guidance
- Navigation rail width: 72-80dp (M3 spec), icons + labels
- Content area: remaining ~1840dp width
- Vertical scrolling with `LazyColumn`, each row is a `LazyRow`

### Card Dimensions (adapted from Android TV grid at 1080p)

| Card Type | Width | Height | Use Case |
|-----------|-------|--------|----------|
| Hero/Featured | 1200dp | 400dp | Top spotlight, auto-advances via HorizontalPager |
| Standard content | 280-320dp | 180-200dp | Workout classes, scenic rides |
| Compact stat card | 200dp | 120dp | Ride history summary |
| App launcher tile | 140dp | 140dp | Streaming apps, settings |
| Quick action pill | 160dp | 56dp | "Just Ride", "Quick Start" buttons |

- Between-card spacing (gutters): 16-20dp
- Row left padding: 24dp (from nav rail edge)
- Row vertical spacing: 24-32dp between row title and next row title
- Safe area margins: 48dp left/right, 24dp top/bottom (from Android TV overscan spec, adapted)

### Hero Area
- Use `HorizontalPager` with `pageSpacing = 16.dp`
- Show 1 full card + ~10% peek of the next card to signal scrollability
- Auto-advance every 8-10 seconds; pause on touch
- Content: featured workout, daily challenge, motivational content
- Overlay gradient (bottom 40%) for text readability over imagery

---

## 2. Typography Scale for Arm's Length Reading

A fitness bike screen is viewed at approximately 2-3 feet (60-90cm) -- farther than a phone but closer than a TV. Scale up from M3 defaults by roughly 1.25-1.5x.

### Recommended Type Scale

| Role | M3 Default | Fitness Bike Recommended | Use Case |
|------|-----------|------------------------|----------|
| Display Large | 57sp | 64-72sp | Workout timer, main stat number |
| Display Medium | 45sp | 52sp | Hero card title |
| Headline Large | 32sp | 40sp | Section headers ("Your Rides") |
| Headline Medium | 28sp | 34sp | Card titles |
| Title Large | 22sp | 28sp | Workout metadata (duration, type) |
| Title Medium | 16sp | 20sp | Secondary labels |
| Body Large | 16sp | 20sp | Descriptions, instructions |
| Body Medium | 14sp | 18sp | Card body text |
| Label Large | 14sp | 18sp | Button text, nav labels |
| Label Medium | 12sp | 16sp | Badges, minor annotations |

**Minimum readable size**: 16sp (never go below this on a fitness screen)

**Font choice**: Use a grotesque/neo-grotesque sans-serif (Inter, Roboto, or a brand typeface). Peloton uses a custom typeface based on this family. Weight contrast matters: use Bold (700) for stats/numbers, Regular (400) for body, Medium (500) for labels.

---

## 3. Primary CTA: "Start Workout"

### Peloton's Approach
- The home screen itself does NOT have a single giant "Start Ride" button
- Instead, every workout card IS the CTA -- tapping any class card goes directly to the pre-ride screen
- A "Just Ride" option (free ride without instructor) lives in the "Experiences" section
- This card-as-CTA pattern reduces friction: see it, tap it, ride it

### Recommended Approach for Bowflex Launcher

**Option A: Persistent Quick Start FAB (recommended)**
```
- Floating action button, bottom-right of content area
- Size: 72dp diameter (larger than M3's 56dp default)
- Label: "RIDE" or "START"
- Background: primary brand color (high contrast)
- Always visible regardless of scroll position
- Tap -> immediate free ride mode (or quick-start dialog)
```

**Option B: Top-of-screen Quick Actions Row**
```
- Horizontal row of pill-shaped buttons immediately below hero
- "Just Ride" | "Scenic Ride" | "Last Workout" | "Quick Start"
- Pill height: 56dp, min width: 140dp
- Primary action gets filled/tonal styling; others get outlined
```

Both approaches should get the user pedaling in 2 taps or fewer from the home screen.

---

## 4. Ride History / Stats Display

### Peloton's Pattern
- Workout history accessed via Profile tab
- Post-ride summary shows: total output (kJ), avg/peak power, avg cadence, avg resistance, duration, heart rate zones, calories
- Power Zone rides show zone breakdown (% time in each zone)
- Recent rides appear in a scrollable list with date, type, duration, and output

### Recommended Implementation

**On Home Screen:**
- Dedicated "Recent Rides" `LazyRow` near the top (2nd or 3rd row)
- Each card shows: date, duration, distance, avg power, calories
- Sparkline or mini bar chart of power over time (if data available)
- Streak counter badge (e.g., "5-day streak") as a standalone card at the row start

**Stats Card Layout:**
```
+----------------------------+
| Mon, Mar 7                 |   <- Label (16sp, secondary color)
| 32:15                      |   <- Display (40sp, primary color)
|                             |
| 245 cal  |  142 avg W      |   <- Title (20sp, two columns)
| 14.2 km  |  82 avg RPM     |
+----------------------------+
```
Width: 240dp, Height: 160dp, corner radius: 16dp

**Dedicated History Screen (from nav rail):**
- `LazyColumn` of ride cards, grouped by week
- Filter chips at top: "All", "This Week", "This Month"
- Each card expandable to show detailed metrics
- Chart section: line graph of output trend over time (use Vico or similar Compose chart library)

---

## 5. Streaming App Integration

### Industry Approach
- **Peloton**: Dedicated "Entertainment" tab in bottom nav; shows tiles for Netflix, YouTube, Disney+, YouTube TV, NBA. Tapping opens the streaming app in a WebView or embedded player. Real-time workout metrics (distance, heart rate, calories) overlay the video as a persistent bottom bar.
- **iFit 2.0**: Netflix and Prime Video icons on the home screen sidebar; tapping opens the respective app. Workout metrics remain visible.

### Recommended Implementation

**Entertainment Row on Home Screen:**
- Horizontal row of app tiles (140x140dp each, with app icon + label)
- Apps to support: YouTube, Netflix, web browser, media player, any sideloaded apps
- Tapping launches the app via `Intent`
- Since this is a jailbroken device, launch real Android apps (not WebViews)

**Workout Metrics Overlay (critical differentiation):**
- When a streaming app is active, show a **persistent floating overlay** (using `SYSTEM_ALERT_WINDOW` or the platform-signed privilege)
- Overlay bar: 1920x80dp, bottom of screen, semi-transparent dark background (80% opacity)
- Shows: elapsed time, distance, RPM, watts, heart rate (if connected), calories
- Tap overlay to expand; swipe down to minimize to a small pill
- This overlay is a separate service/activity -- the SerialBridge already provides the sensor data

**App Management:**
- Long-press app tile for options: "Uninstall", "Move", "Hide"
- "Add App" tile at end of row opens an app picker (installed apps not already shown)

---

## 6. Color Scheme and Contrast

### Peloton's Palette (reference)
- Background: Woodsmoke `#101113` (near-black)
- Text primary: Pumice `#CED0CF` (light gray, NOT pure white -- reduces eye strain)
- Accent: Cardinal `#C41F2F` (energetic red for CTAs and live indicators)
- Contrast ratio: ~12.4:1 (Pumice on Woodsmoke) -- exceeds WCAG AAA

### Recommended Dark Theme Palette

```
Surface:           #121212  (M3 dark surface baseline)
Surface Container: #1E1E1E  (cards, elevated elements)
Surface High:      #2C2C2C  (hover/focus states, active cards)
Surface Bright:    #383838  (modal backgrounds, dialogs)

On Surface:        #E0E0E0  (primary text -- not pure white)
On Surface Variant:#9E9E9E  (secondary text, labels)

Primary:           #FF4444  (brand red -- workout energy)
On Primary:        #FFFFFF
Primary Container: #93000A  (dark red for contained elements)

Secondary:         #4FC3F7  (cool blue -- stats, progress)
On Secondary:      #003547

Tertiary:          #81C784  (green -- success, achievements, active)
On Tertiary:       #003A02

Error:             #FFB4AB
Outline:           #444444  (subtle borders)
```

### Contrast Requirements
- **Normal text (< 24sp)**: minimum 4.5:1 ratio (WCAG AA)
- **Large text (>= 24sp)**: minimum 3:1 ratio
- **UI components** (buttons, icons): minimum 3:1 ratio
- All recommended colors above meet or exceed these ratios against their backgrounds

### Environmental Considerations
- **Bright gym/room**: The dark theme with light text works well; avoid large white areas that cause glare
- **Dim room**: Pure white (#FFFFFF) on dark is too harsh at arm's length; use off-white (#E0E0E0) to reduce eye fatigue
- **Sweaty user**: Higher contrast helps when vision is partially obscured by sweat; avoid thin fonts and low-contrast secondary text during active workout screens
- **Status bar**: Use high-contrast icons (white on dark) for clock, battery, BLE status

---

## 7. Touch Target Sizes for Fitness Equipment

### Research-Based Minimums
- General minimum: 48x48dp (M3 standard) = ~9mm physical
- **Fitness equipment minimum: 56x56dp** (accounts for sweaty fingers, arm's length reach, vibration from pedaling)
- For primary actions during workouts: **72dp minimum** (similar to FAB sizing)
- Spacing between targets: minimum 12dp (prevents accidental taps while exercising)

### Specific Recommendations

| Element | Minimum Size | Spacing |
|---------|-------------|---------|
| Nav rail icons | 56x56dp | 16dp vertical |
| Card touch area | Full card surface | 16dp between cards |
| Buttons (primary) | 56dp height, 140dp min width | 16dp |
| Buttons (during workout) | 72dp height, 160dp min width | 24dp |
| Close/dismiss | 48x48dp | 24dp from edges |
| Slider/seekbar thumb | 32dp diameter, 48dp touch area | N/A |
| Toggle switches | 52x32dp visual, 56x48dp touch | 16dp |

### Interaction Patterns
- **No small text links** -- everything is a tappable card or button
- **No hover states** -- this is a touchscreen, use pressed/focused states instead
- **Generous touch slop** -- increase touch slop to 12dp (default is 8dp) to account for vibration
- **Debounce taps** -- 300ms debounce on action buttons to prevent double-triggers from shaky hands
- **Swipe gestures**: require 40dp minimum travel to register (prevents accidental swipes from pedaling motion)

---

## 8. Jetpack Compose Implementation Notes

### Core Architecture
```kotlin
@Composable
fun FitnessLauncherScreen() {
    Row(modifier = Modifier.fillMaxSize()) {
        // Left navigation rail
        NavigationRail(
            modifier = Modifier.width(80.dp),
            containerColor = MaterialTheme.colorScheme.surface
        ) { /* nav items */ }

        // Main content
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(
                start = 24.dp, end = 48.dp,
                top = 24.dp, bottom = 24.dp
            ),
            verticalArrangement = Arrangement.spacedBy(32.dp)
        ) {
            // Hero pager
            item { HeroPager() }
            // Quick actions
            item { QuickActionRow() }
            // Content rows
            item { ContentRow(title = "Recent Rides", items = recentRides) }
            item { ContentRow(title = "Recommended", items = recommended) }
            item { ContentRow(title = "Entertainment", items = apps) }
        }
    }
}

@Composable
fun ContentRow(title: String, items: List<ContentItem>) {
    Column {
        Text(
            text = title,
            style = MaterialTheme.typography.headlineLarge, // 40sp
            color = MaterialTheme.colorScheme.onSurface
        )
        Spacer(Modifier.height(16.dp))
        LazyRow(
            horizontalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            items(items) { item ->
                ContentCard(item) // 280-320dp wide
            }
        }
    }
}
```

### Key Compose Components
- `HorizontalPager` for hero carousel (with `beyondBoundsPageCount = 1`)
- `LazyRow` for content rows (NOT regular `Row` -- lazy loading matters for images)
- `LazyColumn` for the overall vertical scroll
- `NavigationRail` from M3 for left nav
- `Card` with `tonalElevation` for content cards
- `FloatingActionButton` for the persistent "Ride" button

### Performance Considerations
- Use `Modifier.fillMaxWidth()` judiciously -- avoid recomposition cascades
- Cache images with Coil (`AsyncImage` composable) with disk caching
- Use `remember` / `derivedStateOf` for scroll-dependent UI (e.g., collapsing hero)
- Target 16ms frame time -- the device has only 2GB RAM and an older SoC
- Use `@Stable` annotations on data classes to prevent unnecessary recompositions

### Focus/Accessibility
- Implement proper focus traversal for any D-pad/remote scenarios
- Use `Modifier.focusable()` and `Modifier.onFocusChanged{}` for visual focus rings
- Focus ring: 3dp border, primary color, 4dp offset from element edge
- Announce content descriptions for screen readers (even if unlikely, it is good practice)

---

## 9. Summary of Key Design Principles

1. **Dark theme mandatory** -- reduces glare, looks premium, saves OLED power (if applicable), matches industry standard (Peloton, iFit, every gym equipment maker uses dark themes)

2. **Cards are CTAs** -- every content card is directly tappable to start that activity; no intermediate menus

3. **2-tap maximum** to start any workout from cold boot to pedaling

4. **Minimum 56dp touch targets** -- larger than phone apps, accounting for sweaty hands and vibration

5. **16sp minimum text size** -- nothing smaller on screen, ever

6. **Left nav rail** for primary navigation, not bottom tabs (landscape optimization)

7. **Horizontal row browsing** -- proven by Netflix, Peloton, Android TV; users understand this pattern instantly

8. **Persistent workout metrics overlay** when running third-party apps

9. **High contrast but not harsh** -- off-white text (#E0E0E0) on near-black, colored accents for actions only

10. **Personalization over time** -- show recently used apps and workout types first; learn user preferences

---

## Sources

- [Peloton Homescreen Personalization](https://www.pelobuddy.com/home-screen-bike-personalized/)
- [Peloton Revamping Homescreen with Personalized Rows](https://www.onepeloton.com/press/articles/revamping-peloton-homescreen-experience-with-personalized-rows)
- [Peloton Brand Colors (Mobbin)](https://mobbin.com/colors/brand/peloton)
- [Peloton Entertainment Integration](https://www.onepeloton.com/blog/peloton-entertainment)
- [Peloton App Design Analysis (DesignRush)](https://www.designrush.com/best-designs/apps/peloton-app-design)
- [Peloton UIColors (GitHub)](https://github.com/pelotoncycle/Peloton-UIColors)
- [iFit 2.0 Streaming Integration (T3)](https://www.t3.com/active/watch-netflix-of-prime-video-while-you-workout-with-ifits-new-os)
- [Android TV Layouts (Android Developers)](https://developer.android.com/design/ui/tv/guides/styles/layouts)
- [Android TV Leanback Layouts](https://developer.android.com/training/tv/start/layouts)
- [Material Design 3 Typography](https://m3.material.io/styles/typography/applying-type)
- [Material Design 3 Large Screen Guidance](https://m3.material.io/blog/material-design-for-large-screens)
- [Material Design 3 in Compose](https://developer.android.com/develop/ui/compose/designsystems/material3)
- [Fitness App UX Design Principles (Stormotion)](https://stormotion.io/blog/fitness-app-ux/)
- [Touch Target Size Research (NN/g)](https://www.nngroup.com/articles/touch-target-size/)
- [W3C Touch Target Size Research](https://www.w3.org/WAI/GL/mobile-a11y-tf/wiki/Summary_of_Research_on_Touch/Pointer_Target_Size)
- [WCAG Contrast Requirements](https://webaim.org/articles/contrast/)
- [Material Design Touch Targets](https://m2.material.io/develop/web/supporting/touch-target)
- [Android Font Size Guidelines (LearnUI)](https://www.learnui.design/blog/android-material-design-font-size-guidelines.html)
- [Lazy Rows/Columns for Android TV with Compose (Joe Birch)](https://joebirch.co/android/lazy-columns-and-rows-for-android-tv-with-jetpack-compose/)
