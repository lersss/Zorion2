# v2_redetail.ps1 — повторная детализация принятых (больше деталей, сглаживание).
param([int]$StartSeed = 700)

. "$PSScriptRoot\comfy_api.ps1"

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine blocks at rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$hiresDir = "C:\Zorion2\ai_drafts\hires"

$ships = @(
    @{ Name = "dragonfly"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v8.png" },
    @{ Name = "ghost"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v9.png" },
    @{ Name = "lightbulb"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v1.png" },
    @{ Name = "cigarette"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v2.png" },
    @{ Name = "turtle"; Src = "C:\Zorion2\ai_drafts\processed_concepts\v4.png" }
)

$i = 0
foreach ($s in $ships) {
    $i++
    $seed = $StartSeed + $i
    Write-Host ("=== v2 " + $s.Name + " ===")
    $hires = Join-Path $hiresDir ($s.Name + "_v2_hires.png")
    $r = Send-ComfyHiRes -Prompt $stage2 -InputImage $s.Src -Seed $seed -Steps 40 -Cfg 7.5 -Denoise 0.5 -OutputPath $hires
    if ($r) {
        $clean = Join-Path $hiresDir ($s.Name + "_v2_clean.png")
        & $py "C:\Zorion2\tools\process_ship.py" $hires $clean
    }
}

Write-Host "=== v2 done ==="