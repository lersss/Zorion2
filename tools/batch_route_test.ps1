# batch_route_test.ps1 — ТЕСТОВАЯ пачка спрайтов мини-игры «Прокладка маршрута» (направление A).
# ТЗ: docs/specs/2026-09-25-маршрут-мини-игра-интерфейс.md §4 (3 тестовых ассета:
# star_core 512, beacon 128, false_signal 128). Звезда — txt2img + альфа по яркости;
# маяк/ложный — ControlNet-силуэт (этап 1) + детализация (этап 2) + жёсткий вырез.
# Запуск: powershell -ExecutionPolicy Bypass -File tools\batch_route_test.ps1
param(
    [int]$StarCount = 16,
    [int]$IconSeedsPerForm = 4
)

. "$PSScriptRoot\comfy_api.ps1"

$root     = "C:\Zorion2\ai_drafts\route"
$silDir   = "$root\silhouettes"
$starRaw  = "$root\star_raw";  $starOut  = "$root\star"
$beaconRaw = "$root\beacon_raw"; $beaconOut = "$root\beacon"
$falseRaw = "$root\false_raw";  $falseOut = "$root\false"
New-Item -ItemType Directory -Path $silDir, $starRaw, $starOut, $beaconRaw, $beaconOut, $falseRaw, $falseOut -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$model = "juggernaut-xl-v9.safetensors"
$manifest = @()

# ---------- силуэты маяка/ложного ----------
Write-Host "=== Silhouettes ==="
& $py "C:\Zorion2\tools\make_route_silhouettes.py"

# ---------- 1. star_core (светящаяся, альфа по яркости) ----------
$starNeg = "planet, comet, spacecraft, ring, letters, text, watermark, lens flare streak, color, multiple stars, blurry, low quality, deformed"
$starPrompts = @(
    "a single bright star core with radiating spikes, volumetric glow, soft bloom, perfectly centered, pure white light, grayscale, on pure black background, game sprite, high contrast",
    "a single bright star with four long diffraction spikes, intense white core, soft radial glow, perfectly centered, grayscale, on pure black background, game sprite, high contrast",
    "a single glowing sun star, six radiating rays, soft corona, white light, centered, grayscale, on pure black background, game sprite",
    "a single brilliant star core, soft spherical bloom, faint short rays, pure white, centered, grayscale, on pure black background, game sprite, high contrast"
)

Write-Host "=== star_core: $StarCount ==="
for ($i = 0; $i -lt $StarCount; $i++) {
    $num = 'S{0:d2}' -f ($i + 1)
    $p = $starPrompts[$i % $starPrompts.Count]
    $seed = 71000 + $i
    $raw = "$starRaw\$num.png"
    Write-Host "--- star $num seed=$seed ---"
    $r = Send-ComfyPrompt -Prompt $p -NegativePrompt $starNeg -Width 1024 -Height 1024 `
        -Steps 30 -Cfg 6.0 -Seed $seed -Model $model -OutputPath $raw
    if (-not $r) { Write-Host "STAR FAILED $num"; continue }
    & $py "C:\Zorion2\tools\process_route_glow.py" $raw "$starOut\$num.png"
    $manifest += [pscustomobject]@{
        num = $num; asset = "star_core"; raw = $raw; out = "$starOut\$num.png"
        seed = $seed; prompt = $p; negative = $starNeg
        steps = 30; cfg = 6.0; model = $model
        post = "process_route_glow.py (luminance->alpha, 512)"
    }
}

# ---------- 2. beacon / false_signal (силуэт + детализация + жёсткий вырез) ----------
$beaconNeg = "ship, planet, letters, text, watermark, multiple objects, cartoon, flat vector, blurry, low quality, deformed"
$beaconS1 = "a single small navigation beacon satellite, glowing signal mast, antenna dish, emissive light, neutral grey metal, centered, crisp silhouette, on pure black background, sci-fi game icon, no text, no watermark"
$beaconS2 = "ENTIRE navigation beacon covered with dense mechanical texture, panel lines, greebles, rivets, vents, weathering, subtle gradients, glowing emissive details, neutral grey metal, no orange, no flames, crisp silhouette, maximal detail, masterpiece"

$falseNeg = $beaconNeg
$falseS1 = "a single damaged decoy distress beacon, broken dish, flickering signal, same family as a nav beacon but wrong, neutral grey metal, centered, crisp silhouette, on pure black background, sci-fi game icon, no text, no watermark"
$falseS2 = "ENTIRE damaged decoy beacon covered with dense mechanical texture, cracked panels, broken dish, bent mast, flickering emissive light, weathering, neutral grey metal, no orange, no flames, crisp silhouette, maximal detail, masterpiece"

function Invoke-IconBatch {
    param([string]$Kind, [string[]]$Forms, [string]$S1, [string]$S2, [string]$Neg,
          [string]$RawDir, [string]$OutDir, [string]$Prefix)
    $idx = 0
    foreach ($form in $Forms) {
        for ($k = 0; $k -lt $IconSeedsPerForm; $k++) {
            $idx++
            $num = "$Prefix{0:d2}" -f $idx
            $seed1 = 72000 + $idx + ($(if ($Kind -eq 'beacon') { 0 } else { 500 }))
            $seed2 = 73000 + $idx + ($(if ($Kind -eq 'beacon') { 0 } else { 500 }))
            $sil = "$silDir\$form.png"
            $st1 = "$RawDir\${num}_stage1.png"
            $st2 = "$RawDir\${num}_stage2.png"
            Write-Host "--- $Kind $num ($form) seed1=$seed1 seed2=$seed2 ---"
            $r1 = Send-ComfyControlNet -Prompt $S1 -SilhouettePath $sil -NegativePrompt $Neg `
                -Seed $seed1 -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $st1
            if (-not $r1) { Write-Host "STAGE1 FAILED $num"; continue }
            $r2 = Send-ComfyImg2Img -Prompt $S2 -InputImage $st1 -NegativePrompt $Neg `
                -Seed $seed2 -Steps 40 -Cfg 7.5 -Denoise 0.5 -OutputPath $st2
            if (-not $r2) { Write-Host "STAGE2 FAILED $num"; continue }
            & $py "C:\Zorion2\tools\process_route_icon.py" $st2 "$OutDir\$num.png"
            $script:manifest += [pscustomobject]@{
                num = $num; asset = $Kind; form = $form; raw = $st2; out = "$OutDir\$num.png"
                seed1 = $seed1; seed2 = $seed2
                stage1 = @{ prompt = $S1; negative = $Neg; steps = 40; cfg = 7; strength = 1.5; denoise = 0.85; model = $model; controlnet = "controlnet-canny-sdxl-1.0.safetensors" }
                stage2 = @{ prompt = $S2; negative = $Neg; steps = 40; cfg = 7.5; denoise = 0.5; model = $model }
                post = "process_route_icon.py (hard cutout, 128)"
            }
        }
    }
}

Write-Host "=== beacon ==="
Invoke-IconBatch -Kind "beacon" -Forms @("beacon_01", "beacon_02") -S1 $beaconS1 -S2 $beaconS2 `
    -Neg $beaconNeg -RawDir $beaconRaw -OutDir $beaconOut -Prefix "B"

Write-Host "=== false_signal ==="
Invoke-IconBatch -Kind "false_signal" -Forms @("false_01", "false_02") -S1 $falseS1 -S2 $falseS2 `
    -Neg $falseNeg -RawDir $falseRaw -OutDir $falseOut -Prefix "F"

$json = $manifest | ConvertTo-Json -Depth 8
[System.IO.File]::WriteAllText("$root\manifest.json", $json, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "=== Done: route test batch ($($manifest.Count) assets) ==="
