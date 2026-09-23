# tools/import_ship_sprites_smoke.ps1
# Смоук импорта принятых кораблей (спеки docs/specs/2026-09-21-угол-корабля-в-метаданных.md
# §9 п.18 и docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §6.1 п.1/2/6).
# Работает на temp-копиях: реальные ai_drafts/final_accepted и
# web/static/sprites НЕ трогаются. Проверяет: копирование по хэшу, гвард
# orient_meta (легаси -> Angle 0, Flip false), конфликт имён (без перезаписи),
# идемпотентность (повторный прогон не копирует и не печатает строки), фильтр
# (сироты и race_humans_01..06 не импортируются), печать Race и порядок реестра
# (людской блок первым: starship -> cruiser -> carrier -> fighter).
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

# 7 PNG с РАЗНЫМ содержимым: 5 принятых + конфликт-цель + легаси-люди.
$seen = @{}
$pool = @()
foreach ($f in Get-ChildItem -LiteralPath $spritesSrc -Filter *.png -File) {
    $h = (Get-FileHash -LiteralPath $f.FullName).Hash
    if ($seen.ContainsKey($h)) { continue }
    $seen[$h] = $true
    $pool += $f
    if ($pool.Count -ge 7) { break }
}
if ($pool.Count -lt 7) { Write-Output 'RESULT: FAIL - need >=7 distinct source sprites'; exit 1 }

# Принятые: порядок меты намеренно перемешан (проверяем порядок реестра).
Copy-Item -LiteralPath $pool[0].FullName -Destination (Join-Path $accDir 'race_humans_starship.png')
Copy-Item -LiteralPath $pool[1].FullName -Destination (Join-Path $accDir 'race_humans_starship_02.png')
Copy-Item -LiteralPath $pool[2].FullName -Destination (Join-Path $accDir 'race_humans_starship_03.png')
Copy-Item -LiteralPath $pool[3].FullName -Destination (Join-Path $accDir 'race_humans_cruiser.png')
Copy-Item -LiteralPath $pool[4].FullName -Destination (Join-Path $accDir 'race_humans_cruiser_02.png')
# Конфликт имён: то же имя в игре с ДРУГИМ хэшем — не перезаписывать.
Copy-Item -LiteralPath $pool[5].FullName -Destination (Join-Path $accDir 'race_humans_fighter.png')
Copy-Item -LiteralPath $pool[6].FullName -Destination (Join-Path $sprDir 'race_humans_fighter.png')
# Фильтр: легаси-люди (в мете) и сирота (без записи) импортироваться НЕ должны.
Copy-Item -LiteralPath $pool[6].FullName -Destination (Join-Path $accDir 'race_humans_01.png')
Copy-Item -LiteralPath $pool[5].FullName -Destination (Join-Path $accDir 'race_orphan_99.png')

$meta = @(
    [ordered]@{ file = 'race_humans_starship.png'; race = 'humans'; race_name = 'Humans' },
    [ordered]@{ file = 'race_humans_cruiser_02.png'; race = 'humans'; race_name = 'Humans'; angle = -180; orient_meta = $true },
    [ordered]@{ file = 'race_humans_starship_02.png'; race = 'humans'; race_name = 'Humans'; angle = 20; flip = $true; orient_meta = $true },
    [ordered]@{ file = 'race_humans_cruiser.png'; race = 'humans'; race_name = 'Humans'; angle = 200; orient_meta = $true },
    [ordered]@{ file = 'race_humans_starship_03.png'; race = 'humans'; race_name = 'Humans'; orient_meta = $true },
    [ordered]@{ file = 'race_humans_fighter.png'; race = 'humans'; race_name = 'Humans'; orient_meta = $true },
    [ordered]@{ file = 'race_humans_01.png'; race = 'humans'; race_name = 'Humans' }
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
Check 'copies new file starship' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_starship.png'))
Check 'copies new file starship_02' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_starship_02.png'))
Check 'copies new file starship_03' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_starship_03.png'))
Check 'copies new file cruiser' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_cruiser.png'))
Check 'copies new file cruiser_02' (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_cruiser_02.png'))
Check 'conflict name not overwritten' ((Get-FileHash -LiteralPath (Join-Path $sprDir 'race_humans_fighter.png')).Hash -eq (Get-FileHash -LiteralPath $pool[6].FullName).Hash)
Check 'filter: legacy humans not copied' (-not (Test-Path -LiteralPath (Join-Path $sprDir 'race_humans_01.png')))
Check 'filter: orphan not copied' (-not (Test-Path -LiteralPath (Join-Path $sprDir 'race_orphan_99.png')))
Check 'filter: no row for legacy humans' ($out1 -notmatch 'File: "race_humans_01\.png"')
Check 'legacy guard Angle 0 Flip false' ($out1 -match 'Angle: 0, Flip: false')
Check 'new pair Angle 20 Flip true' ($out1 -match 'Angle: 20, Flip: true')
Check 'angle 200 normalized to -160' ($out1 -match 'Angle: -160, Flip: false')
Check 'angle -180 normalized to 180' ($out1 -match 'Angle: 180, Flip: false')
Check 'race printed in row' ($out1 -match 'Race: "humans"')
Check 'rows printed = 5' (([regex]::Matches($out1, '\{ID:')).Count -eq 5)

# Порядок реестра: людской блок первым (starship -> starship_02 -> starship_03 ->
# cruiser -> cruiser_02), несмотря на перемешанный порядок меты.
$rowLines = @(($out1 -split "`r?`n") | Where-Object { $_ -match '^\s*\{ID:' })
$expectedOrder = @(
    'race_humans_starship.png', 'race_humans_starship_02.png', 'race_humans_starship_03.png',
    'race_humans_cruiser.png', 'race_humans_cruiser_02.png'
)
$orderOk = ($rowLines.Count -eq $expectedOrder.Count)
if ($orderOk) {
    for ($i = 0; $i -lt $expectedOrder.Count; $i++) {
        if ($rowLines[$i] -notmatch [regex]::Escape('File: "' + $expectedOrder[$i] + '"')) { $orderOk = $false; break }
    }
}
Check 'registry order: humans first, meta order within' $orderOk

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
