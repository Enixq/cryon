<#
.SYNOPSIS
    Локальная сборка автономного Android-APK Cryon (без ПК-бэкенда, работает по
    любой сети). Полностью повторяет проверенную последовательность CI
    (.github/workflows/release.yml, job build-android).

.DESCRIPTION
    Шаги (в точности как в CI):
      1. Сборка фронтенда:            frontend/  ->  npm ci + npm run build
      2. Вкладываем в пакет mobile:   frontend/dist  ->  mobile/dist  (для //go:embed)
      3. gomobile bind (arm64):       mobile/  ->  android/app/libs/cryonmobile.aar
      4. Сборка APK:                  android/  ->  gradlew assembleDebug
      5. Копируем результат в dist/ с осмысленным именем и печатаем путь.

    Итог: android/app/build/outputs/apk/debug/app-debug.apk
          и копия dist/Cryon2-<версия>-android-arm64.apk

.PARAMETER Tag
    Метка версии для имени файла (по умолчанию "dev"). В CI это имя git-тега (v0.2.0).

.PARAMETER SkipFrontend
    Пропустить пересборку фронтенда (если dist уже собран и не менялся).

.PARAMETER Release
    Собрать release-APK (assembleRelease) вместо debug. Требует настроенной
    подписи в android/app/build.gradle.kts, иначе Gradle упадёт. По умолчанию debug.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\build-android.ps1

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\build-android.ps1 -Tag v0.2.1

.NOTES
    Предустановка (один раз):
      - Go 1.26+            (go version)
      - Node 20+           (node --version)
      - JDK 17 или 21      (java -version)  -> JAVA_HOME
      - Android SDK + NDK  -> ANDROID_HOME / ANDROID_SDK_ROOT, NDK через SDK Manager
      - gomobile/gobind:
          go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20260908204917-8b95e45f8d3e
          go install golang.org/x/mobile/cmd/gobind@v0.0.0-20260908204917-8b95e45f8d3e
          gomobile init

    БЕЗОПАСНОСТЬ: скрипт НЕ зашивает ключи/секреты в APK. В бэкенд попадает только
    то, что пользователь введёт в зашифрованных настройках на устройстве.
#>
[CmdletBinding()]
param(
    [string]$Tag = "dev",
    [switch]$SkipFrontend,
    [switch]$Release
)

$ErrorActionPreference = "Stop"

# --- Пути. Корень репозитория = родитель каталога scripts/ ---
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot  = Split-Path -Parent $scriptDir
$frontend  = Join-Path $repoRoot "frontend"
$mobile    = Join-Path $repoRoot "mobile"
$android   = Join-Path $repoRoot "android"
$aarPath   = Join-Path $android  "app\libs\cryonmobile.aar"

function Write-Step($msg) { Write-Host "`n=== $msg ===" -ForegroundColor Cyan }
function Fail($msg)       { Write-Host "ОШИБКА: $msg" -ForegroundColor Red; exit 1 }

# Запускает внешнюю команду и падает, если код возврата != 0
# (в PowerShell 5.1 нет оператора && — проверяем $LASTEXITCODE вручную).
function Invoke-Checked($file, [string[]]$cmdArgs, $workDir) {
    Push-Location $workDir
    try {
        & $file @cmdArgs
        if ($LASTEXITCODE -ne 0) { Fail "$file $($cmdArgs -join ' ') → код $LASTEXITCODE" }
    } finally {
        Pop-Location
    }
}

# --- 0. Проверка toolchain ---
Write-Step "Проверка инструментов"
foreach ($tool in @("go", "node", "npm", "java", "gomobile")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Fail "не найден '$tool' в PATH. См. блок NOTES в начале скрипта (предустановка)."
    }
}
if (-not $env:ANDROID_HOME -and -not $env:ANDROID_SDK_ROOT) {
    Write-Host "ВНИМАНИЕ: не заданы ANDROID_HOME / ANDROID_SDK_ROOT — Gradle может не найти Android SDK." -ForegroundColor Yellow
}
Write-Host "go:       $((go version) 2>&1)"
Write-Host "node:     $((node --version) 2>&1)"
Write-Host "gomobile: найден"

# --- 1. Фронтенд ---
if ($SkipFrontend) {
    Write-Step "Фронтенд: пропущен (-SkipFrontend)"
    if (-not (Test-Path (Join-Path $frontend "dist\index.html"))) {
        Fail "frontend/dist не собран, а -SkipFrontend задан. Убери флаг для первой сборки."
    }
} else {
    Write-Step "Сборка фронтенда (npm ci + npm run build)"
    if (Test-Path (Join-Path $frontend "package-lock.json")) {
        Invoke-Checked "npm" @("ci") $frontend
    } else {
        Write-Host "package-lock.json нет — использую npm install" -ForegroundColor Yellow
        Invoke-Checked "npm" @("install") $frontend
    }
    Invoke-Checked "npm" @("run", "build") $frontend
}

# --- 2. Вкладываем dist в пакет mobile (для //go:embed all:dist) ---
Write-Step "Копирование frontend/dist → mobile/dist"
$mobileDist = Join-Path $mobile "dist"
if (Test-Path $mobileDist) { Remove-Item -Recurse -Force $mobileDist }
Copy-Item -Recurse (Join-Path $frontend "dist") $mobileDist
Write-Host "Скопировано в $mobileDist"

# --- 3. gomobile bind → cryonmobile.aar ---
Write-Step "gomobile bind (android/arm64, -tags cryonmobile)"
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $aarPath) | Out-Null
Invoke-Checked "go" @("mod", "tidy") $mobile
# ВАЖНО: тег cryonmobile включает встроенный сервер (mobileserver.go); target arm64 —
# как в CI (меньший APK). gomobile сам ставит GOOS=android → активируется
# platform_android.go, Wails-рантайм в сборку НЕ попадает.
Invoke-Checked "gomobile" @(
    "bind", "-target=android/arm64", "-androidapi", "24",
    "-o", $aarPath, "-tags", "cryonmobile", "."
) $mobile
if (-not (Test-Path $aarPath) -or (Get-Item $aarPath).Length -eq 0) {
    Fail "cryonmobile.aar не создан или пуст — смотри вывод gomobile bind выше."
}
Write-Host "AAR готов: $aarPath ($([math]::Round((Get-Item $aarPath).Length/1MB,1)) МБ)"

# --- 4. Сборка APK через Gradle ---
$gradleTask = if ($Release) { "assembleRelease" } else { "assembleDebug" }
Write-Step "Сборка APK (gradlew $gradleTask)"
$gradlew = Join-Path $android "gradlew.bat"
if (-not (Test-Path $gradlew)) { Fail "не найден $gradlew" }
Invoke-Checked $gradlew @($gradleTask, "--no-daemon") $android

# --- 5. Итоговый путь + копия в dist/ ---
$apkKind = if ($Release) { "release" } else { "debug" }
$apkName = if ($Release) { "app-release.apk" } else { "app-debug.apk" }
$apkPath = Join-Path $android "app\build\outputs\apk\$apkKind\$apkName"
if (-not (Test-Path $apkPath)) { Fail "APK не найден по пути $apkPath" }

$distDir = Join-Path $repoRoot "dist"
New-Item -ItemType Directory -Force -Path $distDir | Out-Null
$outApk = Join-Path $distDir "Cryon2-$Tag-android-arm64.apk"
Copy-Item -Force $apkPath $outApk

Write-Step "ГОТОВО"
Write-Host "APK (оригинал): $apkPath" -ForegroundColor Green
Write-Host "APK (копия):    $outApk" -ForegroundColor Green
Write-Host "Размер:         $([math]::Round((Get-Item $apkPath).Length/1MB,1)) МБ"
Write-Host "`nУстанови на телефон: adb install -r `"$outApk`"  (или перекинь файл и открой на устройстве)."
