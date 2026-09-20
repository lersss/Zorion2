# Живой учёт агентов Zorion. Остановить — Ctrl+C.
param(
  [int]$Port = 8790,
  [string]$Project = "Zorion",
  [switch]$Open
)

$env:PORT = "$Port"
$env:PROJECT = $Project

if ($Open) { Start-Process "http://127.0.0.1:$Port" }

node "$PSScriptRoot\server.mjs"
