$ErrorActionPreference = "Stop"
$sdkRoot = Join-Path $env:USERPROFILE "android-sdk"
$jdkRoot = Join-Path $env:USERPROFILE "jdk17"
$dl = Join-Path $env:USERPROFILE "Downloads\wc-android"
New-Item -ItemType Directory -Force -Path $sdkRoot, $dl | Out-Null

function Download($url, $dest) {
  if (Test-Path $dest) { Write-Host "exists: $dest"; return }
  Write-Host "downloading $url"
  Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $dest -TimeoutSec 900
}

# JDK 17 (zip, no admin required)
$jdkZip = Join-Path $dl "jdk17.zip"
Download "https://api.adoptium.net/v3/binary/latest/17/ga/windows/x64/jdk/hotspot/normal/eclipse" $jdkZip
if (-not (Test-Path (Join-Path $jdkRoot "bin\java.exe"))) {
  if (Test-Path $jdkRoot) { Remove-Item -Recurse -Force $jdkRoot }
  Expand-Archive -Path $jdkZip -DestinationPath $dl -Force
  $inner = Get-ChildItem $dl -Directory | Where-Object { $_.Name -like "jdk-17*" } | Select-Object -First 1
  if (-not $inner) { throw "JDK archive layout unexpected" }
  Move-Item $inner.FullName $jdkRoot
}

# Android command-line tools
$cmdZip = Join-Path $dl "cmdline-tools.zip"
Download "https://dl.google.com/android/repository/commandlinetools-win-11076708_latest.zip" $cmdZip
$cmdDir = Join-Path $sdkRoot "cmdline-tools\latest"
if (-not (Test-Path (Join-Path $cmdDir "bin\sdkmanager.bat"))) {
  $tmp = Join-Path $dl "cmdline-tools-extract"
  if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
  Expand-Archive -Path $cmdZip -DestinationPath $tmp -Force
  New-Item -ItemType Directory -Force -Path (Split-Path $cmdDir -Parent) | Out-Null
  Move-Item (Join-Path $tmp "cmdline-tools") $cmdDir
}

# Gradle
$gradleVersion = "8.11.1"
$gradleZip = Join-Path $dl "gradle-$gradleVersion-bin.zip"
Download "https://services.gradle.org/distributions/gradle-$gradleVersion-bin.zip" $gradleZip
$gradleHome = Join-Path $env:USERPROFILE "gradle-$gradleVersion"
if (-not (Test-Path (Join-Path $gradleHome "bin\gradle.bat"))) {
  Expand-Archive -Path $gradleZip -DestinationPath $env:USERPROFILE -Force
}

$env:JAVA_HOME = $jdkRoot
$env:ANDROID_HOME = $sdkRoot
$env:ANDROID_SDK_ROOT = $sdkRoot
$env:Path = "$jdkRoot\bin;$sdkRoot\cmdline-tools\latest\bin;$gradleHome\bin;" + $env:Path

Write-Host "JAVA_HOME=$env:JAVA_HOME"
& "$jdkRoot\bin\java.exe" -version

# Accept licenses and install SDK packages
$yes = ("y`n" * 50)
$yes | & "$sdkRoot\cmdline-tools\latest\bin\sdkmanager.bat" --sdk_root=$sdkRoot --licenses | Out-Null
& "$sdkRoot\cmdline-tools\latest\bin\sdkmanager.bat" --sdk_root=$sdkRoot "platform-tools" "platforms;android-35" "build-tools;35.0.0"

Write-Host "android sdk ready"
& "$sdkRoot\cmdline-tools\latest\bin\sdkmanager.bat" --sdk_root=$sdkRoot --list_installed
