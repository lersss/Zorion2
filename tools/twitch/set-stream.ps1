# set-stream.ps1 - set Twitch stream title / category / tags / language in one command.
#
# Service tool for the creator's Twitch channel (not part of the game).
# Uses the official Twitch API. Secrets and tokens are stored OUTSIDE the repo,
# in %USERPROFILE%\.zorion-twitch\ (config.json + token.json).
#
# One-time setup and examples: tools/twitch/README.md
#
# Usage:
#   powershell -File tools/twitch/set-stream.ps1 -Login
#   powershell -File tools/twitch/set-stream.ps1 -Get
#   powershell -File tools/twitch/set-stream.ps1 -Title "Разработка Zorion" -Category "Software and Game Development" -Tags "gamedev,indiedev" -Language ru
#   powershell -File tools/twitch/set-stream.ps1 -Title "..." -DryRun

param(
    [switch]$Login,
    [string]$Title,
    [string]$Category,
    [string]$Tags,
    [string]$Language,
    [switch]$Get,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $OutputEncoding = [System.Text.Encoding]::UTF8
} catch { }

# Twitch requires the redirect URI to match exactly. The listener binds 127.0.0.1,
# but the registered URI is http://localhost:3000 (localhost resolves to loopback).
$RedirectUri = "http://localhost:3000"
$ListenHost  = "127.0.0.1"
$ListenPort  = 3000
$AuthBase    = "https://id.twitch.tv/oauth2"
$HelixBase   = "https://api.twitch.tv/helix"
$Scope       = "channel:manage:broadcast"
$LoginWaitMin = 3

$ProfileDir = [Environment]::GetFolderPath("UserProfile")
if (-not $ProfileDir) { $ProfileDir = $env:USERPROFILE }
$ConfigDir  = Join-Path $ProfileDir ".zorion-twitch"
$ConfigPath = Join-Path $ConfigDir "config.json"
$TokenPath  = Join-Path $ConfigDir "token.json"

# Secrets must never land in the repository.
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
if ($ConfigDir -like "$RepoRoot*") {
    Write-Host "[ошибка] Каталог с секретами оказался внутри репозитория ($ConfigDir). Это запрещено; задай USERPROFILE вне репозитория." -ForegroundColor Red
    exit 1
}

function Fail([string]$Message) {
    Write-Host "[ошибка] $Message" -ForegroundColor Red
    exit 1
}

function Write-Utf8Json([string]$Path, $Object) {
    $json = $Object | ConvertTo-Json -Depth 6
    $enc = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $json, $enc)
}

function Read-JsonFile([string]$Path) {
    $raw = [System.IO.File]::ReadAllText($Path, [System.Text.Encoding]::UTF8)
    return ($raw | ConvertFrom-Json)
}

function Get-HttpErrorInfo($ErrorRecord) {
    $resp = $ErrorRecord.Exception.Response
    if ($resp -ne $null) {
        try {
            $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
            $body = $reader.ReadToEnd()
            $reader.Close()
            if (-not $body) { $body = $resp.StatusDescription }
            return [pscustomobject]@{ Status = [int]$resp.StatusCode; Body = $body }
        } catch { }
    }
    return [pscustomobject]@{ Status = $null; Body = $ErrorRecord.Exception.Message }
}

function Get-TwitchErrorMessage($ErrorRecord) {
    $info = Get-HttpErrorInfo $ErrorRecord
    $message = $info.Body
    if ($info.Body) {
        try {
            $parsed = $info.Body | ConvertFrom-Json
            if ($parsed.message) { $message = $parsed.message }
        } catch { }
    }
    if ($info.Status) { return "HTTP $($info.Status): $message" }
    return $message
}

function Invoke-Twitch([string]$Method, [string]$Uri, [hashtable]$Headers, $Body) {
    $params = @{ Method = $Method; Uri = $Uri; TimeoutSec = 30 }
    if ($Headers) { $params.Headers = $Headers }
    if ($null -ne $Body) {
        $params.Body = $Body
        $params.ContentType = "application/json"
    }
    try {
        return Invoke-RestMethod @params
    } catch {
        throw (Get-TwitchErrorMessage $_)
    }
}

function Get-ApiHeaders([string]$ClientId, [string]$AccessToken) {
    return @{
        "Authorization" = "Bearer $AccessToken"
        "Client-Id"     = $ClientId
    }
}

function Get-ClientConfig([switch]$AllowPrompt) {
    if (Test-Path -LiteralPath $ConfigPath) {
        $cfg = Read-JsonFile $ConfigPath
        if (-not $cfg.client_id -or -not $cfg.client_secret) {
            Fail "Файл $ConfigPath повреждён (нет client_id/client_secret). Удали его и запусти -Login заново."
        }
        return $cfg
    }
    if (-not $AllowPrompt) {
        Fail "Нужна авторизация: нет данных приложения ($ConfigPath). Запусти один раз: -Login"
    }

    Write-Host "Первый запуск: нужны данные приложения Twitch."
    Write-Host "Заведи приложение на https://dev.twitch.tv/console/apps и укажи:"
    Write-Host "  OAuth Redirect URL: $RedirectUri"
    Write-Host "  Category: любая (например, Application Integration)"
    Write-Host ""
    $clientId = (Read-Host "Client ID").Trim()
    $secure = Read-Host -AsSecureString "Client Secret"
    $bstr = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $clientSecret = [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
    } finally {
        [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
    if (-not $clientId -or -not $clientSecret) { Fail "client_id и client_secret обязательны." }

    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
    Write-Utf8Json $ConfigPath ([pscustomobject]@{ client_id = $clientId; client_secret = $clientSecret })
    Write-Host "Сохранено: $ConfigPath"
    Write-Host ""
    return (Read-JsonFile $ConfigPath)
}

function Save-Token($TokenResponse, $Previous) {
    $refresh = $TokenResponse.refresh_token
    if (-not $refresh -and $Previous) { $refresh = $Previous.refresh_token }

    $expiresIn = 0
    if ($TokenResponse.expires_in) { $expiresIn = [int]$TokenResponse.expires_in }
    $expiresAt = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds() + $expiresIn

    $scope = @()
    if ($TokenResponse.scope) { $scope = @($TokenResponse.scope) }

    $obj = [pscustomobject]@{
        access_token  = $TokenResponse.access_token
        refresh_token = $refresh
        token_type    = $TokenResponse.token_type
        scope         = $scope
        expires_at    = $expiresAt
        saved_at      = (Get-Date).ToUniversalTime().ToString("o")
    }
    Write-Utf8Json $TokenPath $obj
    return $obj
}

function Get-AccessToken([pscustomobject]$Config, [switch]$NoRefresh) {
    if (-not (Test-Path -LiteralPath $TokenPath)) { return $null }
    $token = Read-JsonFile $TokenPath
    if (-not $token.access_token) { return $null }
    if ($NoRefresh) { return $token }

    $now = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    $expiresAt = 0
    if ($token.expires_at) { $expiresAt = [long]$token.expires_at }
    if ($expiresAt -gt ($now + 60)) { return $token }

    if (-not $token.refresh_token) {
        Fail "Токен истёк и нет refresh_token ($TokenPath). Запусти -Login заново."
    }
    Write-Host "Токен истёк, обновляю..."
    $body = @{
        client_id     = $Config.client_id
        client_secret = $Config.client_secret
        grant_type    = "refresh_token"
        refresh_token = $token.refresh_token
    }
    try {
        $resp = Invoke-RestMethod -Method Post -Uri "$AuthBase/token" -Body $body -ContentType "application/x-www-form-urlencoded" -TimeoutSec 30
    } catch {
        Fail "Не удалось обновить токен: $(Get-TwitchErrorMessage $_). Запусти -Login заново."
    }
    $saved = Save-Token $resp $token
    Write-Host "Токен обновлён."
    return $saved
}

function Start-TwitchLogin([string]$ClientId, [string]$ClientSecret) {
    $listener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Parse($ListenHost), $ListenPort)
    try {
        $listener.Start()
    } catch {
        Fail "Не удалось занять порт $ListenPort для приёма редиректа. Закрой программу, занявшую порт, и повтори -Login."
    }

    $authUrl = "$AuthBase/authorize?client_id=$([uri]::EscapeDataString($ClientId))" +
               "&redirect_uri=$([uri]::EscapeDataString($RedirectUri))" +
               "&response_type=code&scope=$([uri]::EscapeDataString($Scope))&force_verify=true"

    Write-Host "Открываю браузер для авторизации Twitch..."
    Write-Host "Если браузер не открылся, перейди по ссылке:"
    Write-Host $authUrl
    Write-Host ""
    try { Start-Process $authUrl | Out-Null } catch { }

    Write-Host "Жду подтверждения (до $LoginWaitMin минут)..."
    $code = $null
    $errorText = $null
    $deadline = (Get-Date).AddMinutes($LoginWaitMin)
    try {
        while ((Get-Date) -lt $deadline) {
            if (-not $listener.Pending()) {
                Start-Sleep -Milliseconds 200
                continue
            }
            $client = $listener.AcceptTcpClient()
            $stream = $client.GetStream()
            $reader = New-Object System.IO.StreamReader($stream, [System.Text.Encoding]::ASCII)
            $requestLine = $reader.ReadLine()
            $requestPath = "/"
            if ($requestLine -match '^\S+\s+(\S+)') { $requestPath = $Matches[1] }

            $query = ""
            $qIndex = $requestPath.IndexOf("?")
            if ($qIndex -ge 0) { $query = $requestPath.Substring($qIndex + 1) }

            $params = @{}
            foreach ($pair in ($query -split '&')) {
                if (-not $pair) { continue }
                $kv = $pair -split '=', 2
                $key = [uri]::UnescapeDataString($kv[0])
                $val = ""
                if ($kv.Count -ge 2) { $val = [uri]::UnescapeDataString($kv[1]) }
                $params[$key] = $val
            }
            $code = $params["code"]
            $errorText = $params["error_description"]
            if (-not $errorText) { $errorText = $params["error"] }

            $html = "<!doctype html><html><head><meta charset='utf-8'><title>Zorion</title></head>" +
                    "<body style='font-family:sans-serif'><h2>Zorion: Twitch</h2>" +
                    "<p>Авторизация завершена. Вернись в консоль, вкладку можно закрыть.</p></body></html>"
            $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($html)
            $header = "HTTP/1.1 200 OK`r`nContent-Type: text/html; charset=utf-8`r`n" +
                      "Content-Length: $($bodyBytes.Length)`r`nConnection: close`r`n`r`n"
            $headerBytes = [System.Text.Encoding]::ASCII.GetBytes($header)
            $stream.Write($headerBytes, 0, $headerBytes.Length)
            $stream.Write($bodyBytes, 0, $bodyBytes.Length)
            $stream.Flush()
            $client.Close()

            if ($code -or $errorText) { break }
        }
    } finally {
        $listener.Stop()
    }

    if ($errorText) { Fail "Twitch отклонил авторизацию: $errorText" }
    if (-not $code) { Fail "Не дождался кода авторизации (таймаут $LoginWaitMin минут). Повтори -Login." }

    $tokenBody = @{
        client_id     = $ClientId
        client_secret = $ClientSecret
        code          = $code
        grant_type    = "authorization_code"
        redirect_uri  = $RedirectUri
    }
    try {
        $resp = Invoke-RestMethod -Method Post -Uri "$AuthBase/token" -Body $tokenBody -ContentType "application/x-www-form-urlencoded" -TimeoutSec 30
    } catch {
        Fail "Не удалось обменять код на токен: $(Get-TwitchErrorMessage $_)"
    }
    $saved = Save-Token $resp $null
    Write-Host ""
    Write-Host "Готово. Токен сохранён: $TokenPath"
    Write-Host ("Доступ выдан, scope: " + (@($saved.scope) -join ", "))
}

function Get-BroadcasterId([pscustomobject]$Config, [pscustomobject]$Token) {
    $headers = Get-ApiHeaders $Config.client_id $Token.access_token
    try {
        $users = Invoke-Twitch -Method "GET" -Headers $headers -Uri "$HelixBase/users"
    } catch {
        Fail "Twitch не принял токен: $($_.Exception.Message). Если токен отозван — запусти -Login заново."
    }
    if (-not $users.data -or $users.data.Count -eq 0) {
        Fail "Twitch не вернул пользователя для этого токена. Запусти -Login заново."
    }
    return $users.data[0]
}

function Write-Usage {
    Write-Host "Twitch stream setup for Zorion"
    Write-Host ""
    Write-Host "  -Login                 разовая авторизация (откроет браузер)"
    Write-Host "  -Get                   показать текущие заголовок, категорию, теги, язык"
    Write-Host "  -Title `"...`"          заголовок эфира"
    Write-Host "  -Category `"...`"       категория (точное имя игры Twitch)"
    Write-Host "  -Tags `"a,b,c`"          теги через запятую (не более 10)"
    Write-Host "  -Language ru           язык эфира (ISO 639-1)"
    Write-Host "  -DryRun                показать запросы, ничего не отправлять"
    Write-Host ""
    Write-Host "Подробности: tools/twitch/README.md"
}

try {
    if ($Login) {
        $config = Get-ClientConfig -AllowPrompt
        Start-TwitchLogin $config.client_id $config.client_secret
        Write-Host ""
        Write-Host "Теперь можно менять эфир одной командой, примеры в tools/twitch/README.md."
        exit 0
    }

    $hasSettings = $PSBoundParameters.ContainsKey("Title") -or $PSBoundParameters.ContainsKey("Category") -or `
                   $PSBoundParameters.ContainsKey("Tags") -or $PSBoundParameters.ContainsKey("Language")

    if (-not $Get -and -not $DryRun -and -not $hasSettings) {
        Write-Usage
        exit 1
    }

    $config = Get-ClientConfig

    if ($DryRun) {
        $token = Get-AccessToken -Config $config -NoRefresh
        if (-not $token) {
            Fail "Нужна авторизация: нет токена ($TokenPath). Запусти один раз: -Login"
        }

        $wantsCategory = $PSBoundParameters.ContainsKey("Category")
        $display = [ordered]@{}
        if ($PSBoundParameters.ContainsKey("Title")) {
            if (-not $Title) { Fail "-Title пустой." }
            $display["title"] = $Title
        }
        if ($wantsCategory) {
            if (-not $Category) { Fail "-Category пустой." }
            $display["game_id"] = "game_id для категории $Category"
        }
        if ($PSBoundParameters.ContainsKey("Tags")) {
            $tagList = @($Tags -split ',' | ForEach-Object { $_.Trim().ToLower() } | Where-Object { $_ } | Select-Object -Unique)
            if ($tagList.Count -gt 10) { $tagList = @($tagList[0..9]) }
            if ($tagList.Count -gt 0) { $display["tags"] = $tagList }
        }
        if ($PSBoundParameters.ContainsKey("Language")) {
            if (-not $Language) { Fail "-Language пустой." }
            $display["broadcaster_language"] = $Language
        }
        $json = $display | ConvertTo-Json -Compress

        Write-Host "[DryRun] Ничего не отправляется. Были бы отправлены запросы:"
        Write-Host "  1) GET $HelixBase/users"
        Write-Host "       Authorization: Bearer <token>"
        Write-Host "       Client-Id: $($config.client_id)"
        Write-Host "     -> broadcaster_id"
        if ($wantsCategory) {
            Write-Host "  2) GET $HelixBase/games?name=$Category"
            Write-Host "     -> game_id"
            Write-Host "  3) PATCH $HelixBase/channels?broadcaster_id=<broadcaster_id>"
        } else {
            Write-Host "  2) PATCH $HelixBase/channels?broadcaster_id=<broadcaster_id>"
        }
        Write-Host "       Content-Type: application/json"
        Write-Host "       body: $json"
        exit 0
    }

    $token = Get-AccessToken -Config $config
    if (-not $token) {
        Fail "Нужна авторизация: нет токена ($TokenPath). Запусти один раз: -Login"
    }

    $broadcaster = Get-BroadcasterId $config $token
    $broadcasterId = $broadcaster.id

    if ($Get) {
        $headers = Get-ApiHeaders $config.client_id $token.access_token
        $channels = Invoke-Twitch -Method "GET" -Headers $headers -Uri "$HelixBase/channels?broadcaster_id=$broadcasterId"
        if (-not $channels.data -or $channels.data.Count -eq 0) {
            Fail "Twitch не вернул данные канала."
        }
        $ch = $channels.data[0]
        Write-Host "Текущий эфир ($($ch.broadcaster_name)):"
        Write-Host "  Заголовок : $($ch.title)"
        Write-Host "  Категория : $($ch.game_name) (id $($ch.game_id))"
        Write-Host ("  Теги      : " + (@($ch.tags) -join ", "))
        Write-Host "  Язык      : $($ch.broadcaster_language)"
        exit 0
    }

    $headers = Get-ApiHeaders $config.client_id $token.access_token
    $payload = [ordered]@{}
    $resolvedGameName = $null

    if ($PSBoundParameters.ContainsKey("Title")) {
        if (-not $Title) { Fail "-Title пустой." }
        $payload["title"] = $Title
    }
    if ($PSBoundParameters.ContainsKey("Category")) {
        if (-not $Category) { Fail "-Category пустой." }
        $games = Invoke-Twitch -Method "GET" -Headers $headers -Uri "$HelixBase/games?name=$([uri]::EscapeDataString($Category))"
        if (-not $games.data -or $games.data.Count -eq 0) {
            Fail "Категория не найдена: '$Category'. Проверь точное название игры на twitch.tv."
        }
        $payload["game_id"] = $games.data[0].id
        $resolvedGameName = $games.data[0].name
    }
    if ($PSBoundParameters.ContainsKey("Tags")) {
        $tagList = @($Tags -split ',' | ForEach-Object { $_.Trim().ToLower() } | Where-Object { $_ } | Select-Object -Unique)
        if ($tagList.Count -gt 10) {
            Write-Host "Внимание: Twitch допускает не более 10 тегов, лишние отброшены." -ForegroundColor Yellow
            $tagList = @($tagList[0..9])
        }
        if ($tagList.Count -gt 0) { $payload["tags"] = $tagList }
    }
    if ($PSBoundParameters.ContainsKey("Language")) {
        if (-not $Language) { Fail "-Language пустой." }
        $payload["broadcaster_language"] = $Language
    }

    if ($payload.Count -eq 0) {
        Fail "Нечего устанавливать: задай хотя бы один из -Title/-Category/-Tags/-Language."
    }

    $json = $payload | ConvertTo-Json -Compress
    Invoke-Twitch -Method "PATCH" -Headers $headers -Uri "$HelixBase/channels?broadcaster_id=$broadcasterId" -Body $json | Out-Null

    Write-Host "Готово. Обновлены настройки канала $($broadcaster.display_name):"
    if ($payload.Contains("title")) { Write-Host "  Заголовок : $Title" }
    if ($payload.Contains("game_id")) { Write-Host "  Категория : $resolvedGameName" }
    if ($payload.Contains("tags")) { Write-Host ("  Теги      : " + ($tagList -join ", ")) }
    if ($payload.Contains("broadcaster_language")) { Write-Host "  Язык      : $Language" }
    exit 0
} catch {
    Write-Host "[ошибка] $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
