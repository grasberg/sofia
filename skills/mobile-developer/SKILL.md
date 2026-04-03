---
name: mobile-developer
description: "📱 React Native, Flutter, Swift, Kotlin -- native-quality mobile apps. Use this skill whenever the user's task involves mobile, react-native, flutter, swift, kotlin, ios, or any related topic, even if they don't explicitly mention 'Mobile Developer'."
---

# 📱 Mobile Developer

> **Category:** development | **Tags:** mobile, react-native, flutter, swift, kotlin, ios

The right framework depends on the team, timeline, and UX bar. Start with the decision, then go deep. A mediocre native app loses to a polished cross-platform one, and vice versa.

## When to Use

- Choosing between React Native, Flutter, and native (Swift/Kotlin)
- Building cross-platform apps with shared business logic
- Implementing offline-first data sync, push notifications, or deep linking
- Optimizing startup time, list rendering, or battery usage
- Preparing apps for App Store / Google Play submission

## Core Principles

- **Choose the framework before writing code.** React Native vs Flutter vs Native is a team decision, not a tech decision. Wrong choice here costs 6+ months. Use the decision matrix below.
- **Platform conventions are not optional.** iOS users expect swipe-back navigation and bottom tabs. Android users expect the back button and material design. Violating conventions makes the app feel "off" even if users cannot articulate why.
- **Offline-first is an architecture, not a feature.** Design data flow around local-first storage from day one. Bolting on offline support later requires a rewrite.
- **Lists are the #1 performance bottleneck.** FlatList (RN), ListView.builder (Flutter), or RecyclerView (Android) exist for a reason. Rendering 500 items in a ScrollView will cause visible jank on mid-range devices.
- **Secure storage is not optional.** API tokens go in Keychain (iOS) / Keystore (Android), never AsyncStorage or SharedPreferences. One rooted device leaks every user's token.

## Workflow

1. **Decide the framework.** Use the decision matrix. Lock this before sprint 1.
2. **Set up navigation first.** Navigation architecture (stack, tabs, deep links) is hard to change later. Get it right early.
3. **Build one screen end-to-end.** API call, local cache, error state, loading state, empty state. This validates the full data flow.
4. **Add platform-specific polish.** Haptic feedback on iOS, edge-to-edge on Android. Small touches that signal quality.
5. **Test on real devices.** Simulators hide performance problems. Test on a 3-year-old Android phone regularly.
6. **Prepare for store submission.** Build signing, screenshots, privacy manifests (iOS), and store listing early. Last-mile delays kill launch dates.

## Framework Decision Matrix

| Factor | React Native | Flutter | Native (Swift/Kotlin) |
|--------|-------------|---------|----------------------|
| **Team skills** | JS/TS devs | Dart learnable in weeks | Need iOS + Android devs |
| **UI fidelity** | Good (can feel non-native) | Excellent (pixel control) | Perfect (platform widgets) |
| **Code sharing** | ~85% shared | ~90% shared | 0% shared |
| **Startup time** | Moderate (JS bridge) | Fast (compiled AOT) | Fastest |
| **Ecosystem** | Huge npm, but quality varies | Growing, Google-backed | Best native API access |
| **Best for** | Teams with web experience | Custom UI-heavy apps | Performance-critical / hardware-heavy |

**Choose React Native when:** your team knows TypeScript and you want to share code with a web app.
**Choose Flutter when:** you want pixel-perfect custom UI and can invest in learning Dart.
**Choose Native when:** you need deep hardware integration (AR, Bluetooth LE, video processing) or peak performance.

## Examples

### Offline-First Pattern (React Native)

```typescript
// Write to local DB immediately, queue sync for background
async function createTask(task: NewTask) {
  const local = await db.tasks.create(task);                    // 1. Instant local write
  await db.syncQueue.add({ type: "CREATE", entity: "task", data: local }); // 2. Queue sync
  return local; // 3. Background service syncs when online, handles 409 conflicts
}
```

### Platform-Specific Differences

| Behavior | iOS | Android |
|----------|-----|---------|
| Back navigation | Swipe from left edge | Hardware/gesture back button |
| Safe areas | Dynamic Island, home bar | Status bar, nav bar |
| Permissions | Ask once, Settings to reset | Ask each time until "Don't ask again" |
| Background tasks | Limited (BGTaskScheduler) | WorkManager, more flexible |
| Notifications | Must request permission | Auto-granted (until Android 13+) |

## Output Templates

### Mobile Architecture Document

```
# Mobile Architecture: [App Name]

## Overview
[What the app does, target platforms (iOS/Android/both), framework choice and rationale.]

## Stack
| Layer | Technology | Why |
|-------|-----------|-----|
| Framework | [React Native / Flutter / Swift+Kotlin] | [Reason] |
| Navigation | [React Navigation / GoRouter / UIKit] | [Reason] |
| State Management | [Zustand / Riverpod / Combine] | [Reason] |
| Local Storage | [MMKV / Hive / Core Data] | [Reason] |
| Networking | [Axios / Dio / URLSession] | [Reason] |

## Architecture Pattern
[MVVM / Clean Architecture / BLoC -- describe data flow from UI to API and back.]

## Key Screens, Navigation & Offline Strategy
[Screen map with tab/stack structure. Offline strategy: local-first, cache-only, or sync approach.]

[Local-first? Cache-only? What syncs and how conflicts resolve.]

## Platform-Specific Considerations
| Feature | iOS Approach | Android Approach |
|---------|-------------|-----------------|
| [Permissions] | [Details] | [Details] |
| [Background tasks] | [Details] | [Details] |

## Release & Distribution
- **CI/CD:** [Fastlane / GitHub Actions / Codemagic]
- **Beta:** [TestFlight / Firebase App Distribution]
- **Store:** [App Store / Google Play submission plan]
```

### When recommending a framework:

```
## Recommendation: [Framework]

### Why this fits your case
- [Reason tied to their team/timeline/requirements]

### Trade-offs to accept
- [Honest downside and mitigation]

### Project setup
[Specific CLI commands to scaffold the project]

### Key libraries
| Purpose | Library | Why |
|---------|---------|-----|
| Navigation | X | ... |
| State | X | ... |
| Storage | X | ... |
```

## Native Platform Patterns

### SwiftUI State Management

| Property Wrapper | Ownership | Use Case |
|-----------------|-----------|----------|
| `@State` | View owns | View-local value types |
| `@Binding` | Parent owns | Child modifies parent's @State |
| `@StateObject` | View creates & owns | First creation of ObservableObject |
| `@ObservedObject` | Parent owns | View observes, does NOT own |
| `@EnvironmentObject` | App/hierarchy owns | Dependency injection, app-wide shared state |

**Key rule:** `@StateObject` for creation, `@ObservedObject` for observation. Wrong choice causes data loss on re-renders. Inject app-wide state via `.environmentObject()` in the root `App` struct.

### Jetpack Compose State

| API | Survives | Use Case |
|-----|----------|----------|
| `remember { mutableStateOf() }` | Recomposition | UI state within a composable |
| `rememberSaveable { mutableStateOf() }` | Process death | State that must persist across config changes |
| `ViewModel` + `StateFlow` | Lifecycle | Production data -- expose via `collectAsStateWithLifecycle()` |

**Key rule:** Always use `collectAsStateWithLifecycle()` (not `collectAsState()`) to stop collecting when the app is backgrounded.

## App Store Submission Checklist

### Both Platforms
- [ ] App icons in all required sizes (use a single 1024x1024 source)
- [ ] Screenshots for each supported device size (minimum 2, recommended 5)
- [ ] Privacy policy URL (required by both stores)
- [ ] App description, keywords, and category selected
- [ ] Version number and build number incremented
- [ ] All test/debug code removed (no console logs, test APIs, or debug menus)
- [ ] Crash-free rate >99% in beta testing

### iOS (App Store Connect)
- [ ] Privacy nutrition labels completed (data collection declarations)
- [ ] App Tracking Transparency prompt if using IDFA
- [ ] Export compliance (encryption usage) declared
- [ ] In-app purchases configured and tested in sandbox
- [ ] Review guidelines checked ([App Store Review Guidelines](https://developer.apple.com/app-store/review/guidelines/))

### Android (Google Play Console)
- [ ] App signed with upload key (Google manages the signing key)
- [ ] Data safety form completed
- [ ] Content rating questionnaire submitted
- [ ] Target API level meets current Google Play requirement
- [ ] Adaptive icon provided (foreground + background layers)

## Anti-Patterns

- **Using ScrollView for long lists.** FlatList/ListView.builder virtualizes off-screen items. ScrollView renders all children, causing memory spikes and jank on 100+ items.
- **Storing tokens in AsyncStorage/SharedPreferences.** These are unencrypted. Use react-native-keychain or flutter_secure_storage. One jailbroken device = full token exposure.
- **Ignoring the notch and safe areas.** Content hidden behind the Dynamic Island or Android nav bar makes the app look broken. Use SafeAreaView (RN) or SafeArea (Flutter) everywhere.
- **Platform-identical UI.** Use platform-adaptive components where conventions differ. Identical bottom sheets on iOS and Android feel wrong on both.
- **Testing only on simulators.** Real performance only shows on mid-range physical devices. Simulators hide memory, CPU, and thermal issues.

## Capabilities

- mobile
- react-native
- flutter
- ios
- android
- native-apis
