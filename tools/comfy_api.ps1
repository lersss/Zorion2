# ComfyUI API Client для Zorion
# Использование: . .\comfy_api.ps1; Send-ComfyPrompt "top-down spaceship" "C:\Zorion2\web\static\sprites\test.png"

$COMFY_URL = "http://127.0.0.1:8188"

function Test-ComfyReady {
    try {
        $resp = Invoke-RestMethod -Uri "$COMFY_URL/system_stats" -Method Get -TimeoutSec 5
        return $true
    } catch {
        return $false
    }
}

function Get-ComfyModels {
    try {
        $obj = Invoke-RestMethod -Uri "$COMFY_URL/object_info/CheckpointLoaderSimple" -Method Get -TimeoutSec 5
        $models = $obj.CheckpointLoaderSimple.input.required.ckpt_name[0]
        return $models
    } catch {
        Write-Error "ComfyUI not reachable at $COMFY_URL"
        return @()
    }
}

function Send-ComfyPrompt {
    param(
        [Parameter(Mandatory=$true)][string]$Prompt,
        [Parameter(Mandatory=$false)][string]$NegativePrompt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers",
        [Parameter(Mandatory=$false)][int]$Width = 512,
        [Parameter(Mandatory=$false)][int]$Height = 512,
        [Parameter(Mandatory=$false)][int]$Steps = 25,
        [Parameter(Mandatory=$false)][double]$Cfg = 7.0,
        [Parameter(Mandatory=$false)][int]$Seed = -1,
        [Parameter(Mandatory=$false)][string]$Model = "v1-5-pruned-emaonly.safetensors",
        [Parameter(Mandatory=$false)][string]$OutputPath = ""
    )

    # Workflow для txt2img
    $workflow = @{
        "1" = @{
            class_type = "CheckpointLoaderSimple"
            inputs = @{ ckpt_name = $Model }
        }
        "2" = @{
            class_type = "CLIPTextEncode"
            inputs = @{ text = $Prompt; clip = @("1", 1) }
        }
        "3" = @{
            class_type = "CLIPTextEncode"
            inputs = @{ text = $NegativePrompt; clip = @("1", 1) }
        }
        "4" = @{
            class_type = "EmptyLatentImage"
            inputs = @{ width = $Width; height = $Height; batch_size = 1 }
        }
        "5" = @{
            class_type = "KSampler"
            inputs = @{
                seed = $(if ($Seed -ge 0) { $Seed } else { Get-Random -Minimum 1 -Maximum 999999999 })
                steps = $Steps
                cfg = $Cfg
                sampler_name = "euler"
                scheduler = "normal"
                denoise = 1.0
                model = @("1", 0)
                positive = @("2", 0)
                negative = @("3", 0)
                latent_image = @("4", 0)
            }
        }
        "6" = @{
            class_type = "VAEDecode"
            inputs = @{ samples = @("5", 0); vae = @("1", 2) }
        }
        "7" = @{
            class_type = "SaveImage"
            inputs = @{ images = @("6", 0); filename_prefix = "zorion_ship" }
        }
    }

    $body = @{ prompt = $workflow } | ConvertTo-Json -Depth 10

    try {
        $response = Invoke-RestMethod -Uri "$COMFY_URL/prompt" -Method Post -Body $body -ContentType "application/json" -TimeoutSec 30
        $promptId = $response.prompt_id
        if (-not $promptId) {
            Write-Error "No prompt_id: $($response | ConvertTo-Json)"
            return $null
        }
        Write-Host "Submitted prompt_id: $promptId"

        # Ждём завершения
        $outputImage = $null
        for ($i = 0; $i -lt 120; $i++) {
            Start-Sleep -Seconds 2
            try {
                $hist = Invoke-RestMethod -Uri "$COMFY_URL/history/$promptId" -Method Get -TimeoutSec 5
                $entry = $hist.$promptId
                if ($entry -and $entry.status) {
                    if ($entry.status.status_str -eq "success" -or $entry.status.completed) {
                        $images = $entry.outputs."7".images
                        if ($images -and $images.Count -gt 0) {
                            $outputImage = $images[0]
                        }
                        break
                    }
                    if ($entry.status.status_str -eq "error") {
                        Write-Error "ComfyUI error: $($entry.status | ConvertTo-Json)"
                        return $null
                    }
                }
            } catch { }
        }

        if ($outputImage) {
            $imgUrl = "$COMFY_URL/view?filename=$($outputImage.filename)&subfolder=$($outputImage.subfolder)&type=$($outputImage.type)"
            $tmpFile = "$env:TEMP\comfy_out_$([guid]::NewGuid().ToString('N')).png"
            curl.exe -sS -o $tmpFile $imgUrl
            $bytes = [System.IO.File]::ReadAllBytes($tmpFile)
            Remove-Item $tmpFile -Force -ErrorAction SilentlyContinue
            if ($OutputPath) {
                $dir = Split-Path $OutputPath -Parent
                if (!(Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
                [System.IO.File]::WriteAllBytes($OutputPath, $bytes)
                Write-Host "Saved: $OutputPath ($($bytes.Length) bytes)"
                return $OutputPath
            }
            return $bytes
        } else {
            Write-Error "No output image after timeout"
            return $null
        }
    } catch {
        Write-Error "ComfyUI API error: $_"
        return $null
    }
}

function Send-ComfyShipPrompt {
    param(
        [Parameter(Mandatory=$true)][string]$Style,
        [Parameter(Mandatory=$false)][string]$Suffix = "",
        [Parameter(Mandatory=$false)][string]$OutputDir = "C:\Zorion2\web\static\sprites",
        [Parameter(Mandatory=$false)][int]$Seed = -1,
        [Parameter(Mandatory=$false)][int]$Steps = 25,
        [Parameter(Mandatory=$false)][double]$Cfg = 7.0
    )

    $styles = @{
        "organic" = "sci-fi SPACECRAFT, top-down orthographic view, horizontal orientation, nose pointing RIGHT, space ship, NOT aircraft, organic-tech design, bone-like texture, smooth curves, blue glowing eyes at front, decorative ridges along hull, cream beige color, isolated on pure solid black background, game asset, 2D sprite, centered, single ship, no text, no watermark, no planet, no stars"
        "military" = "sci-fi SPACECRAFT, top-down orthographic view, horizontal orientation, nose pointing RIGHT, space ship, NOT aircraft, military warship, heavy armor plating, angular aggressive shape, red navigation lights, dark grey steel hull, panel lines, rivets, isolated on pure solid black background, game asset, 2D sprite, centered, single ship, no text, no watermark, no planet, no stars"
        "sleek" = "sci-fi SPACECRAFT, top-down orthographic view, horizontal orientation, nose pointing RIGHT, space ship, NOT aircraft, sleek racing ship, aerodynamic smooth curves, dark blue hull, glowing cyan accent lines, bird-like silhouette, minimal angles, isolated on pure solid black background, game asset, 2D sprite, centered, single ship, no text, no watermark, no planet, no stars"
        "industrial" = "sci-fi SPACECRAFT, top-down orthographic view, horizontal orientation, nose pointing RIGHT, space ship, NOT aircraft, industrial cargo freighter, two large engines on top, boxy yellow ochre hull, utilitarian design, dark grilles, isolated on pure solid black background, game asset, 2D sprite, centered, single ship, no text, no watermark, no planet, no stars"
    }

    if (-not $styles.ContainsKey($Style)) {
        Write-Error "Unknown style: $Style. Available: $($styles.Keys -join ', ')"
        return
    }

    $prompt = $styles[$Style]
    if ($Suffix) { $prompt = "$prompt, $Suffix" }

    $filePart = $Style
    if ($Seed -ge 0) { $filePart = "${Style}_v$Seed" }
    $outFile = Join-Path $OutputDir "ship_${filePart}_ai.png"
    Write-Host "Generating: $Style (seed=$Seed) -> $outFile"
    Write-Host "Prompt: $prompt"

    $result = Send-ComfyPrompt -Prompt $prompt -Width 512 -Height 512 -Steps $Steps -Cfg $Cfg -Seed $Seed -OutputPath $outFile
    return $result
}

Write-Host "ComfyUI API module loaded. Commands:"
Write-Host "  Test-ComfyReady       - Check if ComfyUI is running"
Write-Host "  Get-ComfyModels        - List available models"
Write-Host "  Send-ComfyPrompt       - Generate image from prompt"
Write-Host "  Send-ComfyShipPrompt   - Generate ship by style (organic/military/sleek/industrial)"
