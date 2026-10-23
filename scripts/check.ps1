#requires -Version 7.0
[CmdletBinding()]
param([string] $LintPath = '')

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$projectRoot = Split-Path -Parent $PSScriptRoot
$cacheRoot = Join-Path $projectRoot '.cache'
$scratch = Join-Path $cacheRoot ('check-' + [guid]::NewGuid().ToString('N'))
$cachePrefix = [IO.Path]::GetFullPath($cacheRoot) + [IO.Path]::DirectorySeparatorChar
if (-not [IO.Path]::GetFullPath($scratch).StartsWith($cachePrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Check scratch directory must be inside the project cache.'
}

function Invoke-Checked {
    param([string] $Command, [string[]] $Arguments)
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command failed with exit code $LASTEXITCODE."
    }
}

Push-Location -LiteralPath $projectRoot
try {
    $env:GOCACHE = Join-Path $cacheRoot 'go/build'
    $env:GOMODCACHE = Join-Path $cacheRoot 'go/mod'
    $env:GOTMPDIR = Join-Path $cacheRoot 'go/tmp'
    New-Item -ItemType Directory -Force $scratch, $env:GOCACHE, $env:GOMODCACHE, $env:GOTMPDIR | Out-Null

    Write-Host 'Checking the pinned pre2 lint and formatting rules...'
    if (-not $LintPath) {
        $lintName = if ($IsWindows) { 'golangci-lint.exe' } else { 'golangci-lint' }
        $localLint = Join-Path $projectRoot ".tools/golangci-lint/2.14.0/$lintName"
        $LintPath = if (Test-Path -LiteralPath $localLint) { $localLint } else { 'golangci-lint' }
    }
    $lintVersion = @(Invoke-Checked $LintPath @('version')) -join ''
    if ($lintVersion -notmatch '\bversion 2\.14\.0(?:\s|$)') { throw "Expected golangci-lint 2.14.0; got: $lintVersion" }
    $env:GOLANGCI_LINT_CACHE = Join-Path $cacheRoot 'lint'
    Invoke-Checked $LintPath @('config', 'verify')
    Invoke-Checked $LintPath @('run', './...')
    Invoke-Checked $LintPath @('fmt', '--diff', './...')

    Write-Host 'Checking Go formatting and vet...'
    $packageDirs = @(Invoke-Checked 'go' @('list', '-f', '{{.Dir}}', './...'))
    $goFiles = @($packageDirs | ForEach-Object { Get-ChildItem -LiteralPath $_ -Filter '*.go' -File } | Select-Object -ExpandProperty FullName)
    $unformatted = @(Invoke-Checked 'gofmt' (@('-l') + $goFiles))
    if ($unformatted.Count -ne 0) {
        throw ('Run gofmt on: ' + ($unformatted -join ', '))
    }
    Invoke-Checked 'go' @('vet', './...')

    Write-Host 'Running tests with exact statement coverage...'
    $profile = Join-Path $scratch 'coverage.out'
    Invoke-Checked 'go' @('test', '-covermode=atomic', "-coverprofile=$profile", './...')
    $blocks = @(Get-Content -LiteralPath $profile | Select-Object -Skip 1)
    if ($blocks.Count -eq 0) { throw 'Coverage profile contains no statements.' }
    $statementCount = [long] 0
    foreach ($block in $blocks) {
        if ($block -notmatch '^\S+\s+(\d+)\s+(\d+)$') { throw "Invalid coverage block: $block" }
        $statementCount += [long] $Matches[1]
        if ([long] $Matches[1] -gt 0 -and [long] $Matches[2] -eq 0) {
            throw "Uncovered statements: $block"
        }
    }
    if ($statementCount -eq 0) { throw 'Coverage profile contains no statements.' }
    Invoke-Checked 'go' @('tool', 'cover', "-func=$profile")

    Write-Host 'Checking exact branch coverage...'
    # gobco 1.3.4 accepts one package at a time and copies its entire module.
    # A source-only snapshot keeps mutable caches and temporary tests outside it.
    $snapshot = Join-Path $scratch 'module'
    New-Item -ItemType Directory $snapshot | Out-Null
    foreach ($name in @('go.mod', 'go.sum', 'cmd', 'internal', 'game')) {
        Copy-Item -LiteralPath (Join-Path $projectRoot $name) -Destination $snapshot -Recurse
    }
    Push-Location -LiteralPath $snapshot
    try {
        foreach ($packageDir in $packageDirs) {
            $packagePath = './' + [IO.Path]::GetRelativePath($projectRoot, $packageDir).Replace('\', '/')
            Write-Host "Branch coverage for $packagePath..."
            $branchOutput = @(& go tool gobco -branch $packagePath 2>&1 | ForEach-Object { $_.ToString() })
            if ($LASTEXITCODE -ne 0) { throw ('gobco failed: ' + ($branchOutput -join [Environment]::NewLine)) }
            $branchOutput | ForEach-Object { Write-Host $_ }
            $summaries = @($branchOutput | Where-Object { $_ -match '^Branch coverage:\s*(\d+)/(\d+)\s*$' })
            if ($summaries.Count -ne 1 -or $summaries[0] -notmatch '^Branch coverage:\s*(\d+)/(\d+)\s*$') {
                throw "gobco must report exactly one branch coverage total for $packagePath."
            }
            # 0/0 is valid for a package containing only data/type declarations.
            if ([long] $Matches[1] -ne [long] $Matches[2]) {
                throw "Incomplete branch coverage for ${packagePath}: $($summaries[0])"
            }
        }
    }
    finally { Pop-Location }

    Write-Host 'Building and checking the standalone executable...'
    $binaryName = if ($IsWindows) { 'gravedigger.exe' } else { 'gravedigger' }
    $binaryDir = Join-Path $projectRoot 'bin'
    New-Item -ItemType Directory -Force $binaryDir | Out-Null
    $binary = Join-Path $binaryDir $binaryName
    Invoke-Checked 'go' @('build', '-trimpath', '-o', $binary, './cmd/gravedigger')
    $help = @(Invoke-Checked $binary @('--help')) -join [Environment]::NewLine
    if ($help -notmatch 'Usage: gravedigger unpack') { throw 'Standalone help output is missing usage.' }
    $version = @(Invoke-Checked $binary @('--version')) -join ''
    if ($version -notmatch '^gravedigger \d+\.\d+\.\d+$') { throw "Invalid standalone version: $version" }

    foreach ($variant in @('softdisk', 'retail')) {
        $source = Join-Path $projectRoot "game/$variant"
        $editable = Join-Path $scratch "$variant-editable"
        $repacked = Join-Path $scratch "$variant-repacked"
        Invoke-Checked $binary @('unpack', $source, $editable)
        Invoke-Checked $binary @('repack', $editable, $repacked)
        $originals = @(Get-ChildItem -LiteralPath $source -File | Where-Object { $_.Extension.ToUpperInvariant() -in @('.DD2', '.EXE') })
        $results = @(Get-ChildItem -LiteralPath $repacked -File)
        if ($originals.Count -eq 0 -or $results.Count -ne $originals.Count) { throw "$variant smoke: output file count differs." }
        foreach ($original in $originals) {
            $result = Join-Path $repacked $original.Name
            if ((Get-FileHash -LiteralPath $original.FullName -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath $result -Algorithm SHA256).Hash) {
                throw "$variant smoke: $($original.Name) differs after unpack/repack."
            }
        }
        Write-Host "$variant standalone roundtrip: $($originals.Count) files byte-exact."
    }
    Write-Host "All checks passed. Built $binary ($version)."
}
finally {
    if (Test-Path -LiteralPath $scratch) { Remove-Item -LiteralPath $scratch -Recurse -Force }
    Pop-Location
}
