# Native DOSBox regression: inspect the game's VGA memory before DOS/shell
# teardown can reload a ROM font. Requires the bundled DOS-debug utility.
param(
    [string]$ToolPath = '',
    [string]$DosBoxPath = '',
    [string]$DebugPath = '',
    [int]$TimeoutSeconds = 20
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem
function Set-XlsxText([string]$Path, [string]$Id, [string]$Text, [switch]$ChangeFirstCharacter) {
    $editableRoot = Split-Path (Split-Path $Path -Parent) -Parent
    $manifest = Get-ChildItem -LiteralPath (Join-Path $editableRoot 'meta') -Filter 'manifest.json' -File -Recurse |
        ForEach-Object { Get-Content -LiteralPath $_.FullName -Raw | ConvertFrom-Json } |
        Where-Object { $_.handler -eq 'ExeTextHandler' } | Select-Object -First 1
    $rowNumber = -1
    for ($i = 0; $i -lt $manifest.metadata.strings.Count; $i++) {
        if ($manifest.metadata.strings[$i].id -eq $Id) { $rowNumber = $i + 1; break }
    }
    if ($rowNumber -lt 1) { throw "Manifest string id missing: $Id" }
    $archive = [IO.Compression.ZipFile]::Open($Path, [IO.Compression.ZipArchiveMode]::Update)
    try {
        $documents = @{}
        foreach ($name in @('xl/sharedStrings.xml', 'xl/worksheets/sheet1.xml')) {
            $reader = [IO.StreamReader]::new($archive.GetEntry($name).Open())
            try { $documents[$name] = [xml]$reader.ReadToEnd() } finally { $reader.Dispose() }
        }
        $strings = $documents['xl/sharedStrings.xml']
        $sheet = $documents['xl/worksheets/sheet1.xml']
        $ns = [Xml.XmlNamespaceManager]::new($strings.NameTable)
        $ns.AddNamespace('s', 'http://schemas.openxmlformats.org/spreadsheetml/2006/main')
        $items = $strings.SelectNodes('/s:sst/s:si', $ns)
        $row = $sheet.SelectSingleNode("//s:row[@r='$rowNumber']", $ns)
        $cell = $row.SelectSingleNode("s:c[@r='B$rowNumber']", $ns)
        if (!$cell) {
            $cell = $sheet.CreateElement('c', $ns.LookupNamespace('s'))
            $cell.SetAttribute('r', "B$rowNumber")
            [void]$row.AppendChild($cell)
        }
        if ($ChangeFirstCharacter) {
            $readText = {
                param($sourceCell)
                if (!$sourceCell) { return '' }
                if ($sourceCell.GetAttribute('t') -eq 's') {
                    $value = $sourceCell.SelectSingleNode('s:v', $ns)
                    return $items[[int]$value.InnerText].InnerText
                }
                return $sourceCell.InnerText
            }
            $effectiveText = & $readText $cell
            if ($effectiveText -eq '') { $effectiveText = & $readText $row.SelectSingleNode("s:c[@r='A$rowNumber']", $ns) }
            $Text = 'X' + $effectiveText.Substring(1)
        }
        # Empty translations share an SST entry. Inline text changes only this B
        # cell and leaves all originals, other translations and font rows intact.
        while ($cell.HasChildNodes) { [void]$cell.RemoveChild($cell.FirstChild) }
        $cell.SetAttribute('t', 'inlineStr')
        $inline = $sheet.CreateElement('is', $ns.LookupNamespace('s'))
        $textNode = $sheet.CreateElement('t', $ns.LookupNamespace('s'))
        $space = $sheet.CreateAttribute('xml', 'space', 'http://www.w3.org/XML/1998/namespace')
        $space.Value = 'preserve'
        [void]$textNode.Attributes.Append($space)
        $textNode.InnerText = $Text
        [void]$inline.AppendChild($textNode)
        [void]$cell.AppendChild($inline)
        $archive.GetEntry('xl/worksheets/sheet1.xml').Delete()
        $writer = [IO.StreamWriter]::new($archive.CreateEntry('xl/worksheets/sheet1.xml').Open(), [Text.UTF8Encoding]::new($false))
        try { $writer.Write($sheet.OuterXml) } finally { $writer.Dispose() }
    }
    finally { $archive.Dispose() }
}
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (!$ToolPath) { $ToolPath = Join-Path $repoRoot 'bin/gravedigger.exe' }
if (!$DosBoxPath) { $DosBoxPath = Join-Path $env:USERPROFILE 'scoop/apps/dosbox-staging/current/dosbox.exe' }
if (!$DebugPath) { $DebugPath = Join-Path (Split-Path $DosBoxPath) 'resources/drives/y/dos/debug.com' }
foreach ($path in @($ToolPath, $DosBoxPath, $DebugPath)) {
    if (!(Test-Path -LiteralPath $path -PathType Leaf)) { throw "Required executable missing: $path" }
}
$workBase = [IO.Path]::GetFullPath((Join-Path $repoRoot '.cache/exit-audit'))
$testRoot = Join-Path $workBase ('native-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $testRoot | Out-Null
$oldVideo, $oldAudio = $env:SDL_VIDEODRIVER, $env:SDL_AUDIODRIVER
$env:SDL_VIDEODRIVER, $env:SDL_AUDIODRIVER = 'dummy', 'dummy'
$jobs = @()
$failures = @()
try {
    foreach ($variant in @('softdisk', 'retail')) { foreach ($scenario in @('baseline', 'font', 'long-text', 'combined')) {
        $caseName = "$variant/$scenario"
        $hasFont = $scenario -in @('font', 'combined')
        $hasLong = $scenario -in @('long-text', 'combined')
        $editable = Join-Path $testRoot ($variant + '-' + $scenario + '-edit')
        $repack = Join-Path $testRoot ($variant + '-' + $scenario)
        & $ToolPath unpack (Join-Path $repoRoot ('game/' + $variant)) $editable | Out-Null
        if ($LASTEXITCODE) { throw "unpack failed: $variant" }
        $xbPath = Join-Path $editable 'translatable/exit_screen.xb'
        $xb = [IO.File]::ReadAllBytes($xbPath)
        if ($xb.Length -ne 8107 -or $xb[10] -ne 2) { throw 'Expected canonical 80x25 XBIN with 8x16 embedded font' }
        if ($hasFont) {
            # Change every font byte; matching an unchanged BIOS font cannot pass.
            for ($i = 0; $i -lt 4096; $i++) { $xb[11 + $i] = $xb[11 + $i] -bxor 0x5a }
            for ($i = 0; $i -lt 2000; $i++) {
                $xb[4107 + 2 * $i] = 0x80 + ($i % 32)
                $xb[4108 + 2 * $i] = 0x1e
            }
            [IO.File]::WriteAllBytes($xbPath, $xb)
        }
        if ($hasLong) {
            $marker = 'GRAVEDIGGER_NATIVE_LONG_TEXT_' + ('A' * 1024) + '_END'
            Set-XlsxText (Join-Path $editable 'translatable/strings.xlsx') 'win_line13' $marker
        }
        if ($scenario -eq 'baseline') {
            # Force raw MZ output through a same-length edit. Its CRT data/heap
            # layout remains identical to the original compressed executable.
            Set-XlsxText (Join-Path $editable 'translatable/strings.xlsx') 'copyright' '' -ChangeFirstCharacter
        }
        & $ToolPath repack $editable $repack | Out-Null
        if ($LASTEXITCODE) { throw "repack failed: $variant" }
        [IO.File]::WriteAllBytes((Join-Path $repack 'EXPECTED.FONT'), [byte[]]$xb[11..4106])
        [IO.File]::WriteAllBytes((Join-Path $repack 'EXPECTED.SCREEN'), [byte[]]$xb[4107..8106])
        Copy-Item -LiteralPath $DebugPath -Destination (Join-Path $repack 'DEBUG.COM')
        $gameExe = if ($variant -eq 'softdisk') { '1.exe' } else { 'mansion.exe' }
        $exe = [IO.File]::ReadAllBytes((Join-Path $repack $gameExe))
        $header = [BitConverter]::ToUInt16($exe, 8) * 16
        $heapFloor = [BitConverter]::ToUInt16($exe, $header + 0x27110 + 0x9c)
        $dsOffset = -1
        if ($hasLong) {
            $fileOffset = [Text.Encoding]::ASCII.GetString($exe).IndexOf($marker, [StringComparison]::Ordinal)
            if ($fileOffset -lt $header) { throw 'Long marker was not appended to the MZ image' }
            $dsOffset = $fileOffset - $header - 0x27110
            $expectedLong = [Text.Encoding]::ASCII.GetBytes($marker + [char]0)
            $heapFloor = [Math]::Max($heapFloor, $dsOffset + $expectedLong.Length)
            [IO.File]::WriteAllBytes((Join-Path $repack 'EXPECTED.LONG'), $expectedLong)
        }
        # CS:06A3 is immediately after the game's 4000-byte exit-screen memcpy,
        # before its key wait and DOS termination. G reports the breakpoint hit.
        $commands = @("n $gameExe", 'l', 'g 6a3', 'r', 'n SCREEN.BIN', 'r bx', '0', 'r cx', 'fa0', 'w b800:0',
            'o 3c4 4', 'o 3c5 6', 'o 3ce 4', 'o 3cf 2', 'o 3ce 5', 'o 3cf 0', 'o 3ce 6', 'o 3cf 4',
            'n FONT.BIN', 'r cx', '2000', 'w a000:0', 'n DS.BIN', 'r bx', '1', 'r cx', '0', 'w ds:0', 'q')
        [IO.File]::WriteAllText((Join-Path $repack 'LIVE.IN'), ($commands -join "`r`n") + "`r`n", [Text.Encoding]::ASCII)
        $arguments = @('--noprimaryconf', '--nolocalconf', '--noautoexec',
            '--set', '"output=texture"', '--set', '"texture_renderer=software"', '--set', '"automount=false"',
            '--set', '"shell_history_file="', '--set', '"mididevice=none"',
            '-c', ('"mount c ' + $repack + '"'), '-c', '"c:"', '-c', '"@echo off"',
            '-c', '"autotype -w 1 -p 0.3 enter space , , enter space , , f1 , , esc , , esc y enter , esc y"',
            '-c', '"debug < LIVE.IN > LIVE.OUT"', '-c', '"exit"')
        $process = Start-Process -FilePath $DosBoxPath -ArgumentList $arguments -WorkingDirectory $repack -WindowStyle Hidden -PassThru `
            -RedirectStandardOutput (Join-Path $repack 'dosbox.stdout') -RedirectStandardError (Join-Path $repack 'dosbox.stderr')
        $jobs += @{ Variant = $caseName; GameVariant = $variant; Scenario = $scenario; HasFont = $hasFont; HasLong = $hasLong;
            HeapFloor = $heapFloor; DSOffset = $dsOffset; Directory = $repack; Process = $process; Started = [DateTime]::UtcNow }
    } }
    foreach ($job in $jobs) {
        $remaining = [Math]::Max(0, [int]($TimeoutSeconds * 1000 - ([DateTime]::UtcNow - $job.Started).TotalMilliseconds))
        if (!$job.Process.WaitForExit($remaining)) {
            Stop-Process -Id $job.Process.Id -Force
            $failures += "$($job.Variant): DOSBox timed out"
            continue
        }
        $trace = [IO.File]::ReadAllText((Join-Path $job.Directory 'LIVE.OUT'))
        if ($job.Process.ExitCode -ne 0 -or $trace -notmatch 'CS=[0-9A-F]{4} IP=06A3') {
            $failures += "$($job.Variant): exit-screen breakpoint was not reached"
            continue
        }
        $screen = [IO.File]::ReadAllBytes((Join-Path $job.Directory 'SCREEN.BIN'))
        $plane = [IO.File]::ReadAllBytes((Join-Path $job.Directory 'FONT.BIN'))
        $ds = [IO.File]::ReadAllBytes((Join-Path $job.Directory 'DS.BIN'))
        $expectedScreen = [IO.File]::ReadAllBytes((Join-Path $job.Directory 'EXPECTED.SCREEN'))
        $expectedFont = [IO.File]::ReadAllBytes((Join-Path $job.Directory 'EXPECTED.FONT'))
        if ($screen.Length -ne 4000 -or $plane.Length -ne 8192 -or $ds.Length -ne 65536) { throw 'Incomplete VGA/data-segment memory dump' }
        $screenDiffs, $fontDiffs = 0, 0
        for ($i = 0; $i -lt 4000; $i++) { if ($screen[$i] -ne $expectedScreen[$i]) { $screenDiffs++ } }
        if ($job.HasFont) { for ($glyph = 0; $glyph -lt 256; $glyph++) {
            for ($row = 0; $row -lt 16; $row++) {
                if ($plane[$glyph * 32 + $row] -ne $expectedFont[$glyph * 16 + $row]) { $fontDiffs++ }
            }
        } }
        $longDiffs = 0
        if ($job.HasLong) {
            $expectedLong = [IO.File]::ReadAllBytes((Join-Path $job.Directory 'EXPECTED.LONG'))
            for ($i = 0; $i -lt $expectedLong.Length; $i++) { if ($ds[$job.DSOffset + $i] -ne $expectedLong[$i]) { $longDiffs++ } }
        }
        $job.RuntimeHeap = [BitConverter]::ToUInt16($ds, 0x9c)
        $job.HeapGrowth = $job.RuntimeHeap - $job.HeapFloor
        $job.PoolPointer = [BitConverter]::ToUInt16($ds, 0xd600)
        $job.PoolHeader = if ($job.PoolPointer -ge 4) { [BitConverter]::ToUInt16($ds, $job.PoolPointer - 4) } else { 0 }
        $job.PoolRequest = ($job.PoolHeader -band 0xfffe) - 4
        $job.PoolAlignment = 16 - ($job.PoolPointer -band 15)
        if ($job.PoolPointer -lt 4 -or !($job.PoolHeader -band 1) -or
            $job.PoolRequest -le $job.PoolAlignment + 4096 -or $job.PoolPointer + $job.PoolRequest -gt 65536) {
            $failures += "$($job.Variant): invalid or undersized MM near pool (would underflow its 4096-byte reservation)"
        }
        if ($job.HasLong -and $job.PoolPointer - 4 -lt $job.DSOffset + $expectedLong.Length -and
            $job.PoolPointer + $job.PoolRequest -gt $job.DSOffset) {
            $failures += "$($job.Variant): MM near pool overlaps the relocated long string"
        }
        $detail = @()
        if ($job.HasFont) { $detail += "VGA plane-2 font differences $fontDiffs/4096" }
        if ($job.HasLong) { $detail += "long-string differences $longDiffs/$($expectedLong.Length)" }
        $detail += ('near heap break {0:X4}, growth {1}' -f $job.RuntimeHeap, $job.HeapGrowth)
        $detail += ('near pool ptr {0:X4}, header {1:X4}, request {2}' -f $job.PoolPointer, $job.PoolHeader, $job.PoolRequest)
        $result = "$($job.Variant): B800 differences $screenDiffs/4000; " + ($detail -join '; ')
        Write-Output $result
        if ($screenDiffs -or $fontDiffs -or $longDiffs) { $failures += $result }
    }
    foreach ($variant in @('softdisk', 'retail')) {
        $baseline = $jobs | Where-Object { $_.GameVariant -eq $variant -and $_.Scenario -eq 'baseline' }
        foreach ($job in ($jobs | Where-Object { $_.GameVariant -eq $variant -and $_.Scenario -ne 'baseline' })) {
            if (!$baseline.ContainsKey('RuntimeHeap') -or !$job.ContainsKey('RuntimeHeap')) { continue }
            # MM_Startup asks coreleft() for the remaining near pool. Moving
            # translated strings may reduce that pool, but must not also lower
            # its upper boundary by moving the stack. Allow allocator alignment.
            $tolerance = if ($job.Scenario -eq 'font') { 0 } else { 16 }
            $heapDelta = $job.RuntimeHeap - $baseline.RuntimeHeap
            if ([Math]::Abs($heapDelta) -gt $tolerance) {
                $failures += ('{0}: near-pool boundary {1:X4} differs from baseline {2:X4} by {3} bytes' -f
                    $job.Variant, $job.RuntimeHeap, $baseline.RuntimeHeap, $heapDelta)
            }
            if ($job.Scenario -eq 'font' -and $job.HeapFloor -ne $baseline.HeapFloor) {
                $failures += "$($job.Variant): far font changed the original near-heap start"
            }
            if ($job.Scenario -eq 'font' -and
                ($job.PoolPointer -ne $baseline.PoolPointer -or $job.PoolRequest -ne $baseline.PoolRequest)) {
                $failures += "$($job.Variant): far font changed the original MM near-pool capacity"
            }
        }
    }
}
finally {
    foreach ($job in $jobs) {
        if (!$job.Process.HasExited) { Stop-Process -Id $job.Process.Id -Force; $job.Process.WaitForExit(1000) | Out-Null }
        $job.Process.Dispose()
    }
    $env:SDL_VIDEODRIVER, $env:SDL_AUDIODRIVER = $oldVideo, $oldAudio
    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    if (!$resolvedTestRoot.StartsWith($workBase + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing cleanup outside audit workspace: $resolvedTestRoot"
    }
    Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
}
if ($failures.Count) { throw ($failures -join "`n") }
