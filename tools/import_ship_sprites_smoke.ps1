# tools/import_ship_sprites_smoke.ps1
# Смоук импорта принятых кораблей (спека docs/specs/2026-09-21-угол-корабля-в-метаданных.md
# §9 п.18). Работает на temp-копиях: реальные ai_drafts/final_accepted и
# web/static/sprites НЕ трогаются. Проверяет: копирование по хэшу, гвард
# orient_meta (легаси -> Angle 0, Flip false), конфликт имён (без перезаписи),
# идемпотентность (повторный прогон не копирует и не печатает строки).
# Вывод ASCII (cp866). Exit 0 — PASS, 1 — FAIL.
# Запуск: powershell -File tools/import_ship_sprites_smoke.ps1
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$repo = Split-Path -Parent $PSScriptRoot
$import = Join-Path $PSScriptRoot 'import_ship_sprites.ps1'
$spritesSrc = Join-Path $repo 'web/static/sprites'

$tmp = Join-Path $env:TEMP ('ship_import_smoke_' + [Guid]::NewGuid().ToString('N'))
$accDir = Join-Path $tmp 'accepted'
$sprDir = Join-Path $tmp 'sprites'
$regFile = Join-Path $tmp 'ship_sprites.go'
New-Item -ItemType Directory -Path $accDir, $sprDir -Force | Out-Null

$pool = @(Get-ChildItem -LiteralPath $spritesSrc -Filter *.png -File | Select-Object -First 6)
if ($pool.Count -lt 6) { Write-Output 'RESULT: FAIL - need >=6 source sprites'; exit 1 }

Copy-Item -LiteralPath $pool[0].FullName -Destination (Join-Path $accDir 'race_humans_01.png')
Copy-Item -LiteralPath $pool[1].FullName -Destination (Join-Path $accDir 'race_humans_02.png')
Copy-Item -LiteralPath $pool[2].FullName -Destination (Join-Path $accDir 'race_humans_03.png')
Copy-Item -LiteralPath $pool[4].FullName -Destination (Join-Path $accDir 'race_humans_04.png')
Copy-Item -LiteralPath $pool[5].FullName -Destination (Join-Path $accDir 'race_humans_05.png')
Copy-Item -LiteralPath $pool[3].FullName -Destination (Join-Path $sprDir 'race_humans_03.png')

$meta = @(
    [ordered]@{ file = 'race_humans_01.png'; race = 'humans'; race_name = 'Humans'; angle = -21; flip = $true },
    [ordered]@{ file = 'race_humans_02.png'; race = 'humans'; race_name = 'Humans'; angle = 20; flip = $true; orient_meta = $true },
    [ordered]@{ file = 'race_humans_03.png'; race = 'humans'; race_name = 'Humans'; orient_meta = $true },
    [ordered]@{ file = 'race_humans_04.png'; race = 'humans'; race_name = 'Humans'; angle = 200; orient_meta = $true },
    [ordered]@{ file = 'race_humans_05.png'; race = 'humans'; race_name = 'Humans'; angle = -180; orient_meta = $true }
)
$meta | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $accDir 'ships_meta.json') -Encoding UTF8

$reg = @'
package models

var ShipSprites = []ShipSprite{}
'@
Set-Content -LiteralPath $regFile -Value $reg -Encoding UTF8

$fail = 0
function Check([string]$name, [bool]$cond) {
    if ($cond) { Write-Output ("PASS " + $name) } else { Write-Output ("FAIL " + $name); $script:fail++ }
}

$out1 = (& $import -AcceptedDir $accDir -SpritesDir $sprDir -Registry $regFile | Out-String)
Check 'copies new file 01' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_01.png'))
Check 'copies new file 02' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_02.png'))
Check 'copies new file 04' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_04.png'))
Check 'copies new file 05' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_05.png'))
Check 'conflict name not overwritten' ((Get-FileHash -LiteralPath (Join-Path $sprDir 'race_humans_03.png')).Hash -eq (Get-FileHash -LiteralPath $pool[3].FullName).Hash)
Check 'legacy guard Angle 0 Flip false' ($out1 -match 'Angle: 0, Flip: false')
Check 'new pair Angle 20 Flip true' ($out1 -match 'Angle: 20, Flip: true')
Check 'angle 200 normalized to -160' ($out1 -match 'Angle: -160, Flip: false')
Check 'angle -180 normalized to 180' ($out1 -match 'Angle: 180, Flip: false')
Check 'rows printed = 4' (([regex]::Matches($out1, '\{ID:')).Count -eq 4)

# Идемпотентность: вставляем напечатанные строки в temp-реестр (как человек), затем
# повторный прогон не должен ничего копировать и не печатать строки заново.
$rows = @(($out1 -split "`r?`n") | Where-Object { $_ -match '^\s*\{ID:' })
Add-Content -LiteralPath $regFile -Value $rows -Encoding UTF8
$snap1 = (Get-ChildItem -LiteralPath $sprDir -File | ForEach-Object { $_.Name + ':' + (Get-FileHash -LiteralPath $_.FullName).Hash }) -join '|'
$out2 = (& $import -AcceptedDir $accDir -SpritesDir $sprDir -Registry $regFile | Out-String)
$snap2 = (Get-ChildItem -LiteralPath $sprDir -File | ForEach-Object { $_.Name + ':' + (Get-FileHash -LiteralPath $_.FullName).Hash }) -join '|'
Check 'idempotent: no new copies' ($snap1 -eq $snap2)
Check 'idempotent: no rows reprinted' (([regex]::Matches($out2, '\{ID:')).Count -eq 0)

Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
Write-Output ('RESULT: ' + $(if ($fail -eq 0) { 'PASS' } else { 'FAIL' }))
exit $(if ($fail -eq 0) { 0 } else { 1 })
