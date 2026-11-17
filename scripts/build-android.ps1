param(
    [string]$ReferenceRoot = $env:PANGUAI_ANDROID_REFERENCE_ROOT,
    [string]$JavaHome = $env:PANGUAI_JAVA_HOME,
    [string]$SdkDirectory = '',
    [string]$GradleExecutable = $env:PANGUAI_GRADLE,
    [switch]$SkipWebBuild,
    [switch]$Offline
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not $ReferenceRoot) { $ReferenceRoot = Join-Path (Split-Path -Parent (Split-Path -Parent $projectRoot)) '全能工作台' }
if (-not $JavaHome) { $JavaHome = if ($env:JAVA_HOME) { $env:JAVA_HOME } else { 'D:/HBuilderX/plugins/amazon-corretto' } }
if (-not $SdkDirectory -and $env:PANGUAI_ANDROID_SDK) { $SdkDirectory = $env:PANGUAI_ANDROID_SDK }
if (-not $SdkDirectory) { $SdkDirectory = Join-Path $ReferenceRoot '.workbench-local/android-sdk' }
$gradle = if ($GradleExecutable) { $GradleExecutable } else { Join-Path $ReferenceRoot '.workbench-local/gradle-8.13/bin/gradle.bat' }
$package = Get-Content -LiteralPath (Join-Path $projectRoot 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$compiler = Join-Path $SdkDirectory 'ndk/28.2.13676358/toolchains/llvm/prebuilt/windows-x86_64/bin/aarch64-linux-android26-clang.cmd'
foreach ($tool in @((Join-Path $JavaHome 'bin/javac.exe'),$gradle,$compiler,(Join-Path $SdkDirectory 'platforms/android-36/android.jar'))) {
    if (-not (Test-Path -LiteralPath $tool)) { throw "Existing build tool missing: $tool. This script does not install host tools." }
}
$names = @('JAVA_HOME','ANDROID_HOME','ANDROID_USER_HOME','GRADLE_USER_HOME','JAVA_TOOL_OPTIONS','PATH','GOCACHE','GOTMPDIR','GOMODCACHE','TEMP','TMP','GOOS','GOARCH','CGO_ENABLED','CC','CGO_LDFLAGS')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
try {
    $cache = Join-Path $projectRoot '.cache'
    $env:JAVA_HOME = $JavaHome
    $env:ANDROID_HOME = $SdkDirectory
    $env:ANDROID_USER_HOME = Join-Path $cache 'android-user'
    $env:GRADLE_USER_HOME = Join-Path $cache 'gradle'
    $env:GOCACHE = Join-Path $cache 'go-build'
    $env:GOTMPDIR = Join-Path $cache 'go-tmp'
    $env:GOMODCACHE = Join-Path $cache 'go-mod'
    $env:TEMP = Join-Path $cache 'android-tmp'
    $env:TMP = $env:TEMP
    $env:PATH = (Join-Path $JavaHome 'bin') + [IO.Path]::PathSeparator + $env:PATH
    foreach ($dir in @($env:ANDROID_USER_HOME,$env:GRADLE_USER_HOME,$env:GOCACHE,$env:GOTMPDIR,$env:GOMODCACHE,$env:TEMP)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    if ($env:HTTPS_PROXY) {
        $uri = [Uri]$env:HTTPS_PROXY
        $env:JAVA_TOOL_OPTIONS = "$env:JAVA_TOOL_OPTIONS -Dhttps.proxyHost=$($uri.Host) -Dhttps.proxyPort=$($uri.Port) -Dhttp.proxyHost=$($uri.Host) -Dhttp.proxyPort=$($uri.Port) -Dhttp.nonProxyHosts=localhost|127.*"
    }
    if (-not $SkipWebBuild) {
        Push-Location (Join-Path $projectRoot 'frontend')
        try { & npm.cmd run build; if($LASTEXITCODE -ne 0){throw 'Frontend build failed'} } finally { Pop-Location }
    }
    $entry = Join-Path $projectRoot 'backend/cmd/android'
    $assetDirectory = [IO.Path]::GetFullPath((Join-Path $entry 'dist'))
    $allowedParent = [IO.Path]::GetFullPath($entry).TrimEnd('\','/') + [IO.Path]::DirectorySeparatorChar
    if (-not $assetDirectory.StartsWith($allowedParent, [StringComparison]::OrdinalIgnoreCase)) { throw 'Embedded asset path escaped the build directory.' }
    if (Test-Path -LiteralPath $assetDirectory) {
        if ((Get-Item -LiteralPath $assetDirectory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Embedded asset directory must not be a link.' }
        Remove-Item -LiteralPath $assetDirectory -Recurse -Force
    }
    New-Item -ItemType Directory -Path $assetDirectory -Force | Out-Null
    Copy-Item -Path (Join-Path $projectRoot 'dist/web/*') -Destination (Join-Path $entry 'dist') -Recurse -Force
    $config = Get-Content -LiteralPath (Join-Path $projectRoot 'data/platform.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    # Only the app protocol configuration is embedded; never account data, host proxy or MCP credentials.
    $runtime = @{app_secret=$config.app_secret;version=$config.version;device_id='generated-on-device';phone_brand='Android'} | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $entry 'runtime-config.json'),$runtime,(New-Object Text.UTF8Encoding($false)))
    $env:GOOS='android'; $env:GOARCH='arm64'; $env:CGO_ENABLED='1'; $env:CC=$compiler
    $env:CGO_LDFLAGS='-Wl,-z,max-page-size=16384'
    $nativeDir=Join-Path $projectRoot 'android/app/src/main/jniLibs/arm64-v8a'
    New-Item -ItemType Directory -Path $nativeDir -Force | Out-Null
    Push-Location (Join-Path $projectRoot 'backend')
    try {
        & go build -buildvcs=false -trimpath -buildmode=c-shared -ldflags "-s -w -X panguai/internal/buildinfo.Version=$($package.version)" -o (Join-Path $nativeDir 'libpanguai.so') ./cmd/android
        if($LASTEXITCODE -ne 0){throw 'Embedded Go build failed'}
    } finally { Pop-Location }
    # Gradle gets a project-local SDK pointer; no global SDK or Gradle configuration is written.
    [IO.File]::WriteAllText((Join-Path $projectRoot 'android/local.properties'),('sdk.dir='+$SdkDirectory.Replace('\','/')),(New-Object Text.UTF8Encoding($false)))
    $gradleArgs = @('-p',(Join-Path $projectRoot 'android'),'--no-daemon','--console=plain','assembleDebug')
    if($Offline){$gradleArgs += '--offline'}
    & $gradle @gradleArgs
    if($LASTEXITCODE -ne 0){throw 'Android package build failed'}
    $release = Join-Path $projectRoot 'release/Android'
    New-Item -ItemType Directory -Path $release -Force | Out-Null
    $target=Join-Path $release "$($package.panguai.displayName)-$($package.version)-debug.apk"
    Copy-Item -LiteralPath (Join-Path $projectRoot 'android/app/build/outputs/apk/debug/app-debug.apk') -Destination $target -Force
    $hashStream = [IO.File]::OpenRead($target)
    $sha256 = [Security.Cryptography.SHA256]::Create()
    try {
        $hash = [BitConverter]::ToString($sha256.ComputeHash($hashStream)).Replace('-','')
        Write-Host "SHA256: $hash"
    } finally {
        $hashStream.Dispose()
        $sha256.Dispose()
    }
    Write-Host "APK ready: $target"
} finally {
    foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}
}
