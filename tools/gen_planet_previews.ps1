# tools/gen_planet_previews.ps1
#
# Генерация картинок планет (режим full, size=big) для монтажа приёмки
# доработки картинки планеты (спека 2026-09-21). Админский JWT генерируется
# напрямую (HS256, role=admin) — пароли тестеров неизвестны.
#
# Использование: powershell -File tools/gen_planet_previews.ps1
# Выход: C:\Users\admin\AppData\Local\Temp\opencode\previews\<name>.png
param(
    [string]$OutDir = "C:\Users\admin\AppData\Local\Temp\opencode\previews"
)

$ErrorActionPreference = "Stop"
$base = "http://127.0.0.1:8080"

# --- Админский JWT (HS256, role=admin) ---
$secret = "dev-secret-change-me-0123456789abcdef0123456789abcdef"
$header = @{ alg = "HS256"; typ = "JWT" } | ConvertTo-Json -Compress
$now = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$payload = @{ user_id = "preview_admin"; role = "admin"; exp = $now + 86400; iat = $now } | ConvertTo-Json -Compress
function B64Url($bytes) { [Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+','-').Replace('/','_') }
$h = B64Url ([Text.Encoding]::UTF8.GetBytes($header))
$p = B64Url ([Text.Encoding]::UTF8.GetBytes($payload))
$hmac = New-Object System.Security.Cryptography.HMACSHA256
$hmac.Key = [Text.Encoding]::UTF8.GetBytes($secret)
$sig = B64Url ($hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes("$h.$p")))
$token = "$h.$p.$sig"
$headers = @{ Authorization = "Bearer $token" }

# --- Планеты: имя -> (id, тип) ---
$planets = [ordered]@{
    "virno_earth"      = @("6d1c8b43-3a32-414b-8758-19e027a57945", "землеподобная")
    "odvilif_ice"      = @("9c6385fe-68e5-4489-8a42-027c38af786b", "ледяная")
    "rinrin_lava"      = @("92f6b517-fe46-4ce3-9568-fc6049e05ea9", "вулканическая")
    "ognemsem_desert"  = @("86c794ae-3e65-4cff-891d-e53632e15470", "пустынная")
    "sununjar_ocean"   = @("f803ef6b-8138-44ff-aca6-319317af32dd", "океаническая")
    "renelar_organic"  = @("5c664d8d-3fbf-49d5-9b54-9c63a269c653", "органик")
    "benurdra_glass"   = @("6aa6974e-af3a-48a4-be0c-be3de8da026d", "стеклянная (Венера-режим)")
    "calmersor_dead"   = @("860784ec-6e41-4d51-80c2-9e24ab4f1232", "мёртвая")
    "nithe_radio"      = @("20211f06-accc-45ab-8e20-b1ef54c0d797", "радиоактивная")
    "ifgriic_rocky"    = @("28359e20-e660-4570-829e-53333ce2cf9e", "скалистая")
    "challaexan_gas1"  = @("b99bc35c-83f7-425b-8ff0-fcb98c5b31bf", "газовый гигант")
    "eisevax_gas2"     = @("1effabcf-7d10-4b46-b725-0edcbd68e9fd", "газовый гигант")
    "obkryam_gas3"     = @("c0eec985-012f-4f87-b163-7f63a375b889", "газовый гигант")
    "silefmel_gas4"    = @("c3175778-a211-45a5-a88b-c918ea7d4293", "газовый гигант")
    "ogirzia_gas5"     = @("8fb9c4b0-3b99-43af-afb4-d9de676acc5e", "газовый гигант")
    "kirhirtal_gas6"   = @("0a7b7e3c-5556-4b8f-aa02-c8da3ad8a4a7", "газовый гигант")
}

New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
foreach ($name in $planets.Keys) {
    $id = $planets[$name][0]
    $out = Join-Path $OutDir "$name.png"
    try {
        Invoke-RestMethod -Uri "$base/api/planet-image?planet_id=$id&size=big" -Headers $headers -TimeoutSec 60 -OutFile $out
        $len = (Get-Item $out).Length
        Write-Output "OK  $name ($($planets[$name][1])) len=$len"
    } catch {
        Write-Output "ERR $name : $($_.Exception.Message)"
    }
}
Write-Output "DONE -> $OutDir"