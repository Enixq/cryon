The foreground regression fixture bundles the production React playback engine,
device synchronization hook, and native JavaScript bridge into a debug WebView.
It uses a local HTTP audio/API fixture instead of loading the ARM-only Go binary
on an x86 emulator. Generated assets are ignored and excluded from release APKs.

After installing frontend dependencies, run from the repository root:

```powershell
node android/prepare-foreground-test.mjs
$env:CRYON_SEPARATE_TEST_APP='1'
$env:CRYON_RELEASE_VERSION='v0.2.24'
android/gradlew.bat -p android assembleDebug assembleDebugAndroidTest
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
adb install -r android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
adb shell am instrument -w ru.cryon.app.yandextest.test/androidx.test.runner.AndroidJUnitRunner
```

Coverage includes foreground state feedback, user pause/resume/seek/next and
source switches, notification controls connected before media loads, rating and
task removal, background radio with the screen off, LAN controls, and handoff.
