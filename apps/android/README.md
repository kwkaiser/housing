# housing for Android

A thin [Capacitor](https://capacitorjs.com) shell around the deployed web app. The APK only loads `HOUSING_APP_URL` (default `https://housing.kwkaiser.io`) in a WebView. The phone needs to reach that URL, so connect to tailscale first.

Sign-in runs in a Custom Tab rather than the WebView, because Google and other identity providers refuse to sign in inside an embedded WebView. The web app opens `/auth/login?app=<challenge>` in the browser, and after the provider signs you in the server sends the browser to `io.kwkaiser.housing://auth` with a one-time code. The app catches that link and posts the code with its PKCE verifier to `/auth/handoff`, which sets the session cookie in the WebView. `task android:deeplink` adds the intent filter for that link from `intent-filter.xml`, since the generated manifest is not checked in.

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

## Releases

Publishing a Gitea release with a `v<version>` tag builds `housing-<version>.apk`, signs it with the release key and attaches it to that release. Install it from the release page, or point [Obtainium](https://github.com/ImranR98/Obtainium) at the repo to get updates.

The release job needs two repository secrets:

- `ANDROID_KEYSTORE_BASE64`: the PKCS12 release keystore, base64 encoded. The key alias is `housing`.
- `ANDROID_KEYSTORE_PASSWORD`: its password, which is also the key's password.

Android only installs an update signed with the same key as the installed app, so keep the keystore. To build a signed APK locally:

```sh
ANDROID_KEYSTORE=path/to/housing-release.p12 ANDROID_KEYSTORE_PASSWORD=... task android:release APK_VERSION=1.2.3
```

The version code is derived from the version as `major * 10000 + minor * 100 + patch`.
