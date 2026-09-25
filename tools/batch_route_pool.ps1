# batch_route_pool.ps1 — догенерация недостающих типов пула мини-игры «Прокладка маршрута».
# ТЗ: docs/specs/2026-09-25-маршрут-мини-игра-интерфейс.md §4: black_hole (3×512),
# hazard_cloud (3×512, grayscale-alpha, полнокадровая фактура), nebula_bg (3×1024,
# grayscale-alpha, полнокадровая туманность). Звезда/маяк/ложный собраны из старой пачки.
# Использование: powershell -ExecutionPolicy Bypass -File tools\batch_route_pool.ps1 -Type black_hole
param(
    [ValidateSet('black_hole','hazard','nebula','all')][string]$Type = 'all',
    [int]$Count = 8
)

. "$PSScriptRoot\comfy_api.ps1"

$root = "C:\Zorion2\ai_drafts\route\pool"
$py = "C:\ComfyUI\venv\Scripts\python.exe"
$model = "juggernaut-xl-v9.safetensors"
$glow = "C:\Zorion2\tools\process_route_glow.py"
$manifest = @()

function Invoke-GlowBatch {
    param([string]$Kind, [string[]]$Prompts, [string]$Neg, [string]$Prefix,
          [string]$RawDir, [string]$OutDir, [int]$Steps, [double]$Cfg,
          [string[]]$PostArgs)
    New-Item -ItemType Directory -Path $RawDir, $OutDir -Force | Out-Null
    for ($i = 0; $i -lt $Prompts.Count; $i++) {
        $num = "$Prefix{0:d2}" -f ($i + 1)
        $seed = 76000 + $i + ($(switch ($Kind) { 'black_hole' { 0 } 'hazard' { 300 } 'nebula' { 600 } }))
        $raw = "$RawDir\$num.png"
        Write-Host "--- $Kind $num seed=$seed ---"
        $r = Send-ComfyPrompt -Prompt $Prompts[$i] -NegativePrompt $Neg -Width 1024 -Height 1024 `
            -Steps $Steps -Cfg $Cfg -Seed $seed -Model $model -OutputPath $raw
        if (-not $r) { Write-Host "$Kind FAILED $num"; continue }
        & $py $glow $raw "$OutDir\$num.png" @PostArgs
        $script:manifest += [pscustomobject]@{
            num = $num; asset = $Kind; raw = $raw; out = "$OutDir\$num.png"
            seed = $seed; prompt = $Prompts[$i]; negative = $Neg
            steps = $Steps; cfg = $Cfg; model = $model; post = ($PostArgs -join ' ')
        }
    }
}

$bhNeg = "star, sun, planet, letters, text, watermark, spacecraft, multiple objects, color, blurry, low quality, deformed"
$bhPrompts = @(
    "a black hole accretion disc, face-on thin glowing ring around a dark void, strong gravitational lensing arcs, centered, pure white light, grayscale, on pure black background, sci-fi game sprite, high contrast",
    "a black hole accretion disc, tilted elliptical glowing disc, bright upper lensing arc above the dark void, centered, pure white, grayscale, on pure black background, sci-fi game sprite, high contrast",
    "a black hole accretion disc with a narrow bright polar jet, glowing thin ring, dark center, centered, pure white, grayscale, on pure black background, sci-fi game sprite",
    "a black hole face-on, extremely thin bright ring, strong gravitational lensing, dark center, minimal, centered, pure white, grayscale, on pure black background, game sprite, high contrast",
    "a black hole, inclined accretion disc, double lensing arcs above and below, glowing ring, centered, pure white, grayscale, on pure black background, game sprite",
    "a black hole with a narrow vertical polar jet of bright light, thin accretion ring, centered, pure white, grayscale, on pure black background, game sprite",
    "a black hole, face-on thick glowing ring with strong lensing, dark central void, centered, pure white, grayscale, on pure black background, game sprite",
    "a black hole, tilted bright accretion disc with a long lensing arc and a faint polar jet, centered, pure white, grayscale, on pure black background, game sprite"
)

$fogNeg = "stars, planets, spacecraft, letters, text, watermark, hard edges, frames, color, cartoon, low quality"
$fogPrompts = @(
    "a wispy dark nebula cloud, swirling turbulent wisps, soft-edged, grayscale, no stars, filling the frame, on pure black background, texture, high detail",
    "a wispy smoke cloud, diagonal dust lanes, layered bands, soft-edged, grayscale, no stars, filling the frame, on pure black background, texture, high detail",
    "fibrous ionized gas filaments, spiral vortex core, soft-edged, grayscale, no stars, filling the frame, on pure black background, texture, high detail",
    "turbulent swirling fog wisps, fine wispy detail, grayscale, no stars, filling the frame, on pure black background, texture",
    "diagonal layered dust bands, soft wispy fog, grayscale, no stars, filling the frame, on pure black background, texture",
    "a spiral vortex of fibrous filaments, soft edged fog, grayscale, no stars, filling the frame, on pure black background, texture",
    "soft billowing smoke cloud, turbulent curls, grayscale, no stars, filling the frame, on pure black background, texture",
    "thin fibrous threads of ionized gas, vortex, grayscale, no stars, filling the frame, on pure black background, texture"
)

$nebNeg = "stars, planets, spacecraft, letters, text, watermark, hard edges, frames, bright core point, color, low quality"
$nebPrompts = @(
    "a large soft nebula cloud filling the frame, broad billowing cloud, gentle inner glow, very soft gradient, grayscale luminance, no stars, no hard edges, on pure black background, background texture",
    "a thin veiled nebula curtain, vertical folds, very soft gradient, grayscale luminance, no stars, no hard edges, filling the frame, on pure black background, background texture",
    "long tendrils and filaments, wispy nebula outskirts, very soft gradient, grayscale, no stars, no hard edges, filling the frame, on pure black background, background texture",
    "a broad billowing nebula cloud, soft inner glow, grayscale luminance, no stars, no hard edges, filling the frame, on pure black background, background texture",
    "thin veiled curtain of nebula, vertical soft folds, grayscale luminance, no stars, filling the frame, on pure black background, background texture",
    "wispy tendrils and filaments, soft nebula outskirts, grayscale, no stars, filling the frame, on pure black background, background texture",
    "soft diffuse nebula haze, broad gentle glow, grayscale, no stars, no hard edges, filling the frame, on pure black background, background texture",
    "a large wispy nebula with long curved filaments, soft edges, grayscale, no stars, filling the frame, on pure black background, background texture"
)

if ($Type -eq 'black_hole' -or $Type -eq 'all') {
    Write-Host "=== black_hole ==="
    Invoke-GlowBatch -Kind 'black_hole' -Prompts $bhPrompts -Neg $bhNeg -Prefix 'BH' `
        -RawDir "$root\bh_raw" -OutDir "$root\bh" -Steps 30 -Cfg 6.0 `
        -PostArgs @('--canvas','512')
}

if ($Type -eq 'hazard' -or $Type -eq 'all') {
    Write-Host "=== hazard_cloud ==="
    Invoke-GlowBatch -Kind 'hazard' -Prompts $fogPrompts -Neg $fogNeg -Prefix 'H' `
        -RawDir "$root\fog_raw" -OutDir "$root\fog" -Steps 30 -Cfg 6.5 `
        -PostArgs @('--full','--canvas','512','--lo','6','--hi','150','--gamma','1.0')
}

if ($Type -eq 'nebula' -or $Type -eq 'all') {
    Write-Output "=== nebula_bg ==="
    Invoke-GlowBatch -Kind 'nebula' -Prompts $nebPrompts -Neg $nebNeg -Prefix 'N' `
        -RawDir "$root\neb_raw" -OutDir "$root\neb" -Steps 30 -Cfg 6.5 `
        -PostArgs @('--full','--canvas','1024','--lo','5','--hi','150','--gamma','1.0')
}

$json = $manifest | ConvertTo-Json -Depth 8
[System.IO.File]::WriteAllText("$root\manifest_$Type.json", $json, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "=== Done: $Type ($($manifest.Count) assets) ==="
