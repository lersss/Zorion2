# hires_accepted.ps1 — Hi-Res апскейл принятых кораблей (2048px + детализация).
# Каждый: апскейл 4x-UltraSharp -> 2048 -> img2img denoise 0.35.
param([int]$StartSeed = 600)

. "$PSScriptRoot\comfy_api.ps1"

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine blocks at rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

# Принятые корабли: имя -> исходник (200x200 processed) -> имя результата
$accepted = @(
    @{ Name = "dragonfly"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v8.png" },
    @{ Name = "ghost"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v9.png" },
    @{ Name = "lightbulb"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v1.png" },
    @{ Name = "cigarette"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v2.png" },
    @{ Name = "turtle"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v4.png" },
    @{ Name = "flower"; Src = "C:\Zorion2\ai_drafts\processed_concepts\flower_clean.png" },
    @{ Name = "snowflake"; Src = "C:\Zorion2\ai_drafts\processed_concepts2\v10.png" },
    @{ Name = "horseshoe"; Src = "C:\Zorion2\ai_drafts\horseshoe_fixed_clean.png" }
)

$outDir = "C:\Zorion2\ai_drafts\hires"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
$py = "C:\ComfyUI\venv\Scripts\python.exe"

$i = 0
foreach ($ship in $accepted) {
    $i++
    $seed = $StartSeed + $i
    Write-Host "=== Hi-Res $($ship.Name) ==="
    $hires = "$outDir\$($ship.Name)_hires.png"
    $r = Send-ComfyHiRes -Prompt $stage2 -InputImage $ship.Src -Seed $seed -Steps 35 -Cfg 7 -Denoise 0.35 -OutputPath $hires
    if ($r) {
        $clean = "$outDir\$($ship.Name)_clean.png"
        & $py "C:\Zorion2\tools\process_ship.py" $hires $clean
    }
}

Write-Host "=== Hi-Res done: $outDir ==="