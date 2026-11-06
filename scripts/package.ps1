param([string]$Output = "dist", [string]$Go = "go", [string]$Version = "0.1.0", [switch]$All)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
$taskOldOS, $taskOldArch, $taskOldCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
try {
    New-Item -ItemType Directory -Force -Path $Output | Out-Null
    $taskOutput = (Resolve-Path -LiteralPath $Output).Path
    $taskTargets = if ($All) { @('windows/amd64','linux/amd64','darwin/arm64') } else { @((& $Go env GOOS) + '/' + (& $Go env GOARCH)) }
    foreach ($taskTarget in $taskTargets) {
        $env:GOOS, $env:GOARCH = $taskTarget.Split('/')
        $env:CGO_ENABLED = '0'
        $taskName = 'game-l10n-updater_' + $Version + '_' + $env:GOOS + '_' + $env:GOARCH
        if ($taskName -notmatch '^[A-Za-z0-9._-]+$') { throw 'Invalid package name/version' }
        $taskStage = Join-Path $taskOutput ('stage-' + [guid]::NewGuid().ToString('N'))
        $taskBundle = Join-Path $taskStage $taskName
        New-Item -ItemType Directory -Path $taskBundle -Force | Out-Null
        $taskResolvedStage = (Resolve-Path -LiteralPath $taskStage).Path
        if ((Split-Path -Parent $taskResolvedStage) -ne $taskOutput -or (Split-Path -Leaf $taskResolvedStage) -notmatch '^stage-[0-9a-f]{32}$') { throw 'Unsafe stage directory' }
        try {
            $taskExt = if ($env:GOOS -eq 'windows') { '.exe' } else { '' }
            & $Go build -trimpath -buildvcs=false -ldflags '-s -w' -o (Join-Path $taskBundle ('gamel10n' + $taskExt)) ./cmd/gamel10n
            if ($LASTEXITCODE -ne 0) { throw "Build failed for $taskTarget" }
            Copy-Item -LiteralPath README.md,LICENSE,THIRD_PARTY_NOTICES.md -Destination $taskBundle
            foreach ($taskTree in @('docs','examples','godot','LICENSES')) {
                if (-not (Test-Path -LiteralPath $taskTree)) { continue }
                foreach ($taskFile in Get-ChildItem -LiteralPath $taskTree -Recurse -File) {
                    $taskRelative = $taskFile.FullName.Substring($taskRoot.Length + 1)
                    if ($taskRelative -match '(^|[\\/])\.godot([\\/]|$)' -or $taskRelative -match '\.(import|translation|uid)$') { continue }
                    $taskDestination = Join-Path $taskBundle $taskRelative
                    New-Item -ItemType Directory -Path (Split-Path -Parent $taskDestination) -Force | Out-Null
                    Copy-Item -LiteralPath $taskFile.FullName -Destination $taskDestination
                }
            }
            if ($env:GOOS -eq 'windows') {
                Compress-Archive -LiteralPath $taskBundle -DestinationPath (Join-Path $taskOutput ($taskName+'.zip')) -Force
            } else {
                & python (Join-Path $PSScriptRoot 'package_tar.py') $taskBundle (Join-Path $taskOutput ($taskName+'.tar.gz'))
                if ($LASTEXITCODE -ne 0) { throw 'tar failed' }
            }
            Write-Host "Packaged $taskTarget"
        } finally { Remove-Item -LiteralPath $taskResolvedStage -Recurse -Force }
    }
} finally {
    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $taskOldOS, $taskOldArch, $taskOldCGO
    Pop-Location
}
