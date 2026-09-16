# batch_gen.ps1 — массовая генерация 10 РАЗНЫХ кораблей разных стилей.
# Каждый вариант: случайный стиль из 6, двухэтапный процесс (форма + детализация).
param(
    [int]$Count = 10,
    [int]$StartSeed = 100
)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "arrow" = "sci-fi spaceship, top-down view, cream ivory metal hull, glass cockpit at the front nose, steel grey wings with light tips, bronze tail fins, small fins near wings, antenna at nose, dark engine blocks with orange exhaust at the rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "cruiser" = "sci-fi spaceship, top-down view, heavy cruiser, wide massive hull with central tower superstructure, glass cockpit at front, steel armor plates, bronze stern, dark engine battery with orange exhaust at rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "hawk" = "sci-fi spaceship, top-down view, flying wing fighter, HUGE swept wings spanning almost full width, narrow body, glass cockpit, bronze wing tips, thin tail, dark engines with orange exhaust at rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "freighter" = "sci-fi spaceship, top-down view, industrial freighter, boxy hull, glass cockpit at front, TWO HUGE round dark engines at rear with orange exhaust, bronze stern frame, steel ribs, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "protoss" = "sci-fi spaceship, top-down view, elegant alien protoss vessel, golden hull, symmetric diamond-wing shape, glowing blue-purple energy core, curved blades, purple accents, high-tech, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "zerg" = "sci-fi spaceship, top-down view, organic bio-alien zerg creature, chitinous shell, fleshy segments, horns and fangs at front, poison green glowing eyes, organic curved wings, dark chitin, asymmetrical organic, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
}

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing cockpit, rich color palette, no orange, no flames, neutral engine blocks at rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$silMap = @{
    "arrow" = "sil_arrow_modules.png"
    "cruiser" = "sil_cruiser_modules.png"
    "hawk" = "sil_hawk_modules.png"
    "freighter" = "sil_freighter_modules.png"
    "protoss" = "sil_protoss.png"
    "zerg" = "sil_zerg.png"
}

$styles = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\batch_mix"
$procDir = "C:\Zorion2\ai_drafts\processed_batch_mix"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"

for ($i = 0; $i -lt $Count; $i++) {
    $style = $styles[$i % $styles.Count]
    $seed = $StartSeed + $i
    $num = $i + 1
    $silPath = "C:\Zorion2\ai_drafts\silhouettes\$($silMap[$style])"
    $msg = "=== Variant $num/$Count [$style] seed=$seed ==="
    Write-Host $msg
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"
    $clean = "$procDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$style] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $s1
    if (-not $r1) { Write-Host "Stage1 failed v$num"; continue }

    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Write-Host "Stage2 failed v$num"; continue }

    & $py "C:\Zorion2\tools\process_ship.py" $fin $clean
}

$done = "=== Done: $Count ships in $procDir ==="
Write-Host $done