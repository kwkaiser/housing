# housing for Android

A thin [Capacitor](https://capacitorjs.com) shell around the deployed web app. The APK only loads `HOUSING_APP_URL` (default `https://housing.kwkaiser.io`) in a WebView, so sign-in is the same OIDC flow as in the browser and the server sets its usual session cookie. The phone needs to reach that URL, so connect to tailscale first.

The Android project under `android/` is generated from `capacitor.config.ts` and is not checked in. Change the app through the config instead of editing it.

## Build

The Android SDK and JDK come from the `android` dev shell:

```sh
nix develop .#android
task android:build            # debug APK in apps/android/android/app/build/outputs/apk/debug
task android:install-device   # build and install on a phone connected over adb
task android:clean            # delete the generated project
```

`task android:build` generates the project on first use. To install over USB, turn on USB debugging in the phone's developer options and accept the prompt when `adb devices` first sees it. To debug the WebView, open `chrome://inspect` in desktop Chrome while the app is running.

Bumping `@capacitor/android` can change the SDK versions the generated project expects. Keep `platformVersions` and `buildToolsVersions` in `flake.nix` in step with `android/variables.gradle` and the Android Gradle plugin.
