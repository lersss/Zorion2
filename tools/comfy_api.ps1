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
        [Parameter(Mandatory=$false)][string]$NegativePrompt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, perspective, three-quarter view, isometric, 3D render, depth, foreshortening, nose toward viewer, facing camera, tilted, angled view, diagonal view, side view, front view, vanishing point, depth of field",
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

function Send-ComfyControlNet {
    param(
        [Parameter(Mandatory=$true)][string]$Prompt,
        [Parameter(Mandatory=$true)][string]$SilhouettePath,
        [Parameter(Mandatory=$false)][string]$NegativePrompt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers",
        [Parameter(Mandatory=$false)][int]$Width = 1024,
        [Parameter(Mandatory=$false)][int]$Height = 1024,
        [Parameter(Mandatory=$false)][int]$Steps = 30,
        [Parameter(Mandatory=$false)][double]$Cfg = 6.0,
        [Parameter(Mandatory=$false)][int]$Seed = -1,
        [Parameter(Mandatory=$false)][string]$Model = "juggernaut-xl-v9.safetensors",
        [Parameter(Mandatory=$false)][string]$ControlNet = "controlnet-canny-sdxl-1.0.safetensors",
        [Parameter(Mandatory=$false)][double]$Strength = 2.0,
        [Parameter(Mandatory=$false)][double]$Denoise = 0.8,
        [Parameter(Mandatory=$false)][double]$CNEnd = 0.5,
        [Parameter(Mandatory=$false)][string]$OutputPath = ""
    )

    # Загрузка силуэта как изображения (LoadImage требует имя файла в input-папке).
    # Копируем силуэт во входную папку ComfyUI, чтобы LoadImage его увидел.
    $comfyInput = "C:\ComfyUI\input"
    $silName = [System.IO.Path]::GetFileName($SilhouettePath)
    Copy-Item $SilhouettePath "$comfyInput\$silName" -Force

    $workflow = @{
        "1" = @{ class_type = "CheckpointLoaderSimple"; inputs = @{ ckpt_name = $Model } }
        "2" = @{ class_type = "CLIPTextEncode"; inputs = @{ text = $Prompt; clip = @("1", 1) } }
        "3" = @{ class_type = "CLIPTextEncode"; inputs = @{ text = $NegativePrompt; clip = @("1", 1) } }
        "4" = @{ class_type = "VAEEncode"; inputs = @{ pixels = @("8", 0); vae = @("1", 2) } }
        "8" = @{ class_type = "LoadImage"; inputs = @{ image = $silName } }
        "9" = @{
            class_type = "Canny"
            inputs = @{
                image = @("8", 0)
                low_threshold = 0.2
                high_threshold = 0.5
            }
        }
        "10" = @{ class_type = "ControlNetLoader"; inputs = @{ control_net_name = $ControlNet } }
        "11" = @{
            class_type = "ControlNetApply"
            inputs = @{
                conditioning = @("2", 0)
                control_net = @("10", 0)
                image = @("9", 0)
                strength = $Strength
            }
        }
        "5" = @{
            class_type = "KSampler"
            inputs = @{
                seed = $(if ($Seed -ge 0) { $Seed } else { Get-Random -Minimum 1 -Maximum 999999999 })
                steps = $Steps
                cfg = $Cfg
                sampler_name = "dpmpp_2m"
                scheduler = "karras"
                denoise = $Denoise
                model = @("1", 0)
                positive = @("11", 0)
                negative = @("3", 0)
                latent_image = @("4", 0)
            }
        }
        "6" = @{ class_type = "VAEDecode"; inputs = @{ samples = @("5", 0); vae = @("1", 2) } }
        "7" = @{ class_type = "SaveImage"; inputs = @{ images = @("6", 0); filename_prefix = "zorion_ship" } }
    }

    $body = @{ prompt = $workflow } | ConvertTo-Json -Depth 10

    try {
        $response = Invoke-RestMethod -Uri "$COMFY_URL/prompt" -Method Post -Body $body -ContentType "application/json" -TimeoutSec 30
        $promptId = $response.prompt_id
        if (-not $promptId) {
            Write-Error "No prompt_id: $($response | ConvertTo-Json)"
            return $null
        }
        Write-Host "Submitted ControlNet prompt_id: $promptId"

        $outputImage = $null
        for ($i = 0; $i -lt 180; $i++) {
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

function Send-ComfyImg2Img {
    param(
        [Parameter(Mandatory=$true)][string]$Prompt,
        [Parameter(Mandatory=$true)][string]$InputImage,
        [Parameter(Mandatory=$false)][string]$NegativePrompt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, flat, plain, smooth, featureless",
        [Parameter(Mandatory=$false)][int]$Steps = 30,
        [Parameter(Mandatory=$false)][double]$Cfg = 7.0,
        [Parameter(Mandatory=$false)][double]$Denoise = 0.45,
        [Parameter(Mandatory=$false)][int]$Seed = -1,
        [Parameter(Mandatory=$false)][string]$Model = "juggernaut-xl-v9.safetensors",
        [Parameter(Mandatory=$false)][string]$OutputPath = ""
    )

    $comfyInput = "C:\ComfyUI\input"
    $imgName = [System.IO.Path]::GetFileName($InputImage)
    Copy-Item $InputImage "$comfyInput\$imgName" -Force

    $workflow = @{
        "1" = @{ class_type = "CheckpointLoaderSimple"; inputs = @{ ckpt_name = $Model } }
        "2" = @{ class_type = "CLIPTextEncode"; inputs = @{ text = $Prompt; clip = @("1", 1) } }
        "3" = @{ class_type = "CLIPTextEncode"; inputs = @{ text = $NegativePrompt; clip = @("1", 1) } }
        "8" = @{ class_type = "LoadImage"; inputs = @{ image = $imgName } }
        "4" = @{ class_type = "VAEEncode"; inputs = @{ pixels = @("8", 0); vae = @("1", 2) } }
        "5" = @{
            class_type = "KSampler"
            inputs = @{
                seed = $(if ($Seed -ge 0) { $Seed } else { Get-Random -Minimum 1 -Maximum 999999999 })
                steps = $Steps
                cfg = $Cfg
                sampler_name = "dpmpp_2m"
                scheduler = "karras"
                denoise = $Denoise
                model = @("1", 0)
                positive = @("2", 0)
                negative = @("3", 0)
                latent_image = @("4", 0)
            }
        }
        "6" = @{ class_type = "VAEDecode"; inputs = @{ samples = @("5", 0); vae = @("1", 2) } }
        "7" = @{ class_type = "SaveImage"; inputs = @{ images = @("6", 0); filename_prefix = "zorion_ship" } }
    }

    $body = @{ prompt = $workflow } | ConvertTo-Json -Depth 10

    try {
        $response = Invoke-RestMethod -Uri "$COMFY_URL/prompt" -Method Post -Body $body -ContentType "application/json" -TimeoutSec 30
        $promptId = $response.prompt_id
        if (-not $promptId) {
            Write-Error "No prompt_id: $($response | ConvertTo-Json)"
            return $null
        }
        Write-Host "Submitted img2img prompt_id: $promptId"

        $outputImage = $null
        for ($i = 0; $i -lt 180; $i++) {
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

function Send-ComfyHiRes {
    param(
        [Parameter(Mandatory=$true)][string]$Prompt,
        [Parameter(Mandatory=$true)][string]$InputImage,
        [Parameter(Mandatory=$false)][string]$NegativePrompt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers",
        [Parameter(Mandatory=$false)][int]$Steps = 30,
        [Parameter(Mandatory=$false)][double]$Cfg = 7.0,
        [Parameter(Mandatory=$false)][double]$Denoise = 0.35,
        [Parameter(Mandatory=$false)][int]$Seed = -1,
        [Parameter(Mandatory=$false)][string]$Model = "juggernaut-xl-v9.safetensors",
        [Parameter(Mandatory=$false)][string]$Upscaler = "4x-UltraSharp.pth",
        [Parameter(Mandatory=$false)][string]$OutputPath = ""
    )

    $comfyInput = "C:\ComfyUI\input"
    $imgName = [System.IO.Path]::GetFileName($InputImage)
    Copy-Item $InputImage "$comfyInput\$imgName" -Force

    $workflow = @{
        "1" = @{ class_type = "CheckpointLoaderSimple"; inputs = @{ ckpt_name = $Model } }
        "2" = @{ class_type = "CLIPTextEncode"; inputs = @{ text = $Prompt; clip = @("1", 1) } }
        "3" = @{ class_type = "CLIPTextEncode"; inputs = @{ text = $NegativePrompt; clip = @("1", 1) } }
        "8" = @{ class_type = "LoadImage"; inputs = @{ image = $imgName } }
        "12" = @{ class_type = "UpscaleModelLoader"; inputs = @{ model_name = $Upscaler } }
        "13" = @{ class_type = "ImageUpscaleWithModel"; inputs = @{ upscale_model = @("12", 0); image = @("8", 0) } }
        "14" = @{ class_type = "ImageScale"; inputs = @{ image = @("13", 0); width = 2048; height = 2048; upscale_method = "lanczos"; crop = "disabled" } }
        "4" = @{ class_type = "VAEEncode"; inputs = @{ pixels = @("14", 0); vae = @("1", 2) } }
        "5" = @{
            class_type = "KSampler"
            inputs = @{
                seed = $(if ($Seed -ge 0) { $Seed } else { Get-Random -Minimum 1 -Maximum 999999999 })
                steps = $Steps
                cfg = $Cfg
                sampler_name = "dpmpp_2m"
                scheduler = "karras"
                denoise = $Denoise
                model = @("1", 0)
                positive = @("2", 0)
                negative = @("3", 0)
                latent_image = @("4", 0)
            }
        }
        "6" = @{ class_type = "VAEDecode"; inputs = @{ samples = @("5", 0); vae = @("1", 2) } }
        "7" = @{ class_type = "SaveImage"; inputs = @{ images = @("6", 0); filename_prefix = "zorion_hires" } }
    }

    $body = @{ prompt = $workflow } | ConvertTo-Json -Depth 10

    try {
        $response = Invoke-RestMethod -Uri "$COMFY_URL/prompt" -Method Post -Body $body -ContentType "application/json" -TimeoutSec 30
        $promptId = $response.prompt_id
        if (-not $promptId) {
            Write-Error "No prompt_id: $($response | ConvertTo-Json)"
            return $null
        }
        Write-Host "Submitted hires prompt_id: $promptId"

        $outputImage = $null
        for ($i = 0; $i -lt 240; $i++) {
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
        [Parameter(Mandatory=$false)][string]$OutputDir = "C:\Zorion2\ai_drafts",
        [Parameter(Mandatory=$false)][int]$Seed = -1,
        [Parameter(Mandatory=$false)][int]$Steps = 30,
        [Parameter(Mandatory=$false)][double]$Cfg = 6.0,
        [Parameter(Mandatory=$false)][string]$Silhouette = ""
    )

    $styles = @{
        "organic" = "flat 2D top-down game sprite of a sci-fi SPACECRAFT, seen from DIRECTLY ABOVE, orthographic projection, bird's eye view, no depth, ship is HORIZONTAL, its nose pointing to the RIGHT edge of the image, perfectly flat, no perspective, organic-tech design, bone-like texture, smooth curves, blue glowing eyes at front, decorative ridges along hull, cream beige color, on pure solid black background, game asset, centered, single ship, no text, no watermark, no planet, no stars, highly detailed"
        "military" = "flat 2D top-down game sprite of a sci-fi SPACECRAFT, seen from DIRECTLY ABOVE, orthographic projection, bird's eye view, no depth, ship is HORIZONTAL, its nose pointing to the RIGHT edge of the image, perfectly flat, no perspective, military warship, heavy armor plating, angular aggressive shape, red navigation lights, dark grey steel hull, panel lines, rivets, on pure solid black background, game asset, centered, single ship, no text, no watermark, no planet, no stars, highly detailed"
        "sleek" = "flat 2D top-down game sprite of a sci-fi SPACECRAFT, seen from DIRECTLY ABOVE, orthographic projection, bird's eye view, no depth, ship is HORIZONTAL, its nose pointing to the RIGHT edge of the image, perfectly flat, no perspective, sleek racing ship, aerodynamic smooth curves, dark blue hull, glowing cyan accent lines, bird-like silhouette, minimal angles, on pure solid black background, game asset, centered, single ship, no text, no watermark, no planet, no stars, highly detailed"
        "industrial" = "flat 2D top-down game sprite of a sci-fi SPACECRAFT, seen from DIRECTLY ABOVE, orthographic projection, bird's eye view, no depth, ship is HORIZONTAL, its nose pointing to the RIGHT edge of the image, perfectly flat, no perspective, industrial cargo freighter, two large engines on top, boxy yellow ochre hull, utilitarian design, dark grilles, on pure solid black background, game asset, centered, single ship, no text, no watermark, no planet, no stars, highly detailed"
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

    $result = Send-ComfyPrompt -Prompt $prompt -Width 1024 -Height 1024 -Steps $Steps -Cfg $Cfg -Seed $Seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $outFile
    return $result
}

Write-Host "ComfyUI API module loaded. Commands:"
Write-Host "  Test-ComfyReady       - Check if ComfyUI is running"
Write-Host "  Get-ComfyModels        - List available models"
Write-Host "  Send-ComfyPrompt       - Generate image from prompt"
Write-Host "  Send-ComfyShipPrompt   - Generate ship by style (organic/military/sleek/industrial)"
