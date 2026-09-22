@echo off
REM ENCODING: cp866 (OEM). Do not re-save this file as UTF-8 - cmd.exe breaks on it.
chcp 866 >nul
setlocal

REM ===========================================================================
REM  Zorion - поднять все локальные сервера одной командой (двойной клик).
REM  Повторный запуск безопасен: то, что уже работает, пропускается.
REM  Флаг --no-pause: не ждать нажатия клавиши в конце (для запуска из скриптов).
REM ===========================================================================

REM --- что поднимать: 1 = да, 0 = пропустить ---------------------------------
set "COMFY=1"
set "STUDIO=1"
set "DASH=1"

REM --- пути ------------------------------------------------------------------
set "PG_BIN=C:\pgsql\pgsql\bin"
set "PG_DATA=C:\pgsql\data"
set "PG_LOG=C:\pgsql\pg.log"
set "COMFY_DIR=C:\ComfyUI"

set "NOPAUSE=0"
if "%~1"=="--no-pause" set "NOPAUSE=1"

cd /d "%~dp0"
if not exist "logs" mkdir "logs"
title Zorion - запуск серверов

echo.
echo ============== Zorion: запуск серверов ==============
echo.

call :pg
call :redis
call :game
if "%COMFY%"=="1" (call :comfy) else (echo [4/6] Генерация картинок - выключено в настройках)
if "%DASH%"=="1" (call :dash) else (echo [5/6] Учёт агентов - выключено в настройках)
if "%STUDIO%"=="1" (call :studio) else (echo [6/6] Арт-студия - выключено в настройках)

call :summary
call :the_end
exit /b 0


REM ======================== служебные процедуры ==============================

:pg_ready
"%PG_BIN%\pg_isready.exe" -h 127.0.0.1 -p 5432 >nul 2>&1
exit /b %ERRORLEVEL%

:wait_pg
for /l %%i in (1,1,%1) do (
  call :pg_ready
  if not errorlevel 1 exit /b 0
  ping -n 2 127.0.0.1 >nul
)
exit /b 1

:portcheck
netstat -ano -p tcp | findstr /I "LISTENING" | findstr /C:":%1 " >nul 2>&1
exit /b %ERRORLEVEL%

:wait_port
for /l %%i in (1,1,%2) do (
  call :portcheck %1
  if not errorlevel 1 exit /b 0
  ping -n 2 127.0.0.1 >nul
)
exit /b 1


REM ============================ сервера =====================================

:pg
echo [1/6] База данных, порт 5432
call :pg_ready
if not errorlevel 1 (
  echo       уже работает
  exit /b 0
)
echo       поднимаю...
start "PostgreSQL" /min "%PG_BIN%\pg_ctl.exe" start -D "%PG_DATA%" -l "%PG_LOG%"
call :wait_pg 30
if errorlevel 1 (
  echo       НЕ ДОЖДАЛИСЬ запуска - смотри %PG_LOG%
  exit /b 1
)
echo       поднята
exit /b 0

:redis
echo [2/6] Redis, порт 6379
sc query Redis 2>nul | findstr /I "RUNNING" >nul
if not errorlevel 1 (
  echo       уже работает
  exit /b 0
)
echo       поднимаю...
net start Redis >nul 2>&1
if errorlevel 1 (
  echo       НЕ УДАЛОСЬ поднять службу - нужны права администратора
  exit /b 1
)
echo       поднят
exit /b 0

:game
echo [3/6] Игровой сервер, порт 8080
call :portcheck 8080
if not errorlevel 1 (
  echo       уже работает
  exit /b 0
)
echo       поднимаю, первая сборка может занять до минуты...
start "Zorion game" /min cmd /c "go run ./cmd/server >> logs\server_stdout.log 2>> logs\server_stderr.log"
call :wait_port 8080 90
if errorlevel 1 (
  echo       НЕ ПОДНЯЛСЯ за 90 с - смотри logs\server_stderr.log
  exit /b 1
)
echo       поднят
exit /b 0

:comfy
echo [4/6] Генерация картинок, порт 8188
call :portcheck 8188
if not errorlevel 1 (
  echo       уже работает
  exit /b 0
)
echo       поднимаю, загрузка модели ~30 с...
start "ComfyUI" /min /d "%COMFY_DIR%" cmd /c "venv\Scripts\python.exe main.py --port 8188 >> comfy_log.txt 2>&1"
call :wait_port 8188 120
if errorlevel 1 (
  echo       НЕ ПОДНЯЛСЯ за 120 с - смотри %COMFY_DIR%\comfy_log.txt
  exit /b 1
)
echo       поднят
exit /b 0

:dash
echo [5/6] Учёт агентов, порт 8790
call :portcheck 8790
if not errorlevel 1 (
  echo       уже работает
  exit /b 0
)
echo       поднимаю...
start "Учёт агентов" /min cmd /c "set PORT=8790&& set PROJECT=Zorion&& node tools\agent-dash\server.mjs >> logs\agent_dash.out.log 2>&1"
call :wait_port 8790 30
if errorlevel 1 (
  echo       НЕ ПОДНЯЛСЯ за 30 с - смотри logs\agent_dash.out.log
  exit /b 1
)
echo       поднят
exit /b 0

:studio
echo [6/6] Арт-студия, порт 8798
call :portcheck 8798
if not errorlevel 1 (
  echo       уже работает
  exit /b 0
)
echo       поднимаю...
start "Арт-студия" /min cmd /c "go run ./cmd/art-studio >> logs\art_studio.out.log 2>&1"
call :wait_port 8798 90
if errorlevel 1 (
  echo       НЕ ПОДНЯЛАСЬ за 90 с - смотри logs\art_studio.out.log
  exit /b 1
)
echo       поднята
exit /b 0


REM ============================== итог ======================================

:report
call :portcheck %1
if errorlevel 1 (
  echo   [нет]  %~2 - порт %1
) else (
  echo   [ок]   %~2 - порт %1
)
exit /b 0

:summary
echo.
echo ===================== Что получилось =====================
call :report 5432 "База данных"
call :report 6379 "Redis"
call :report 8080 "Игровой сервер"
call :report 8188 "Генерация картинок"
call :report 8790 "Учёт агентов"
call :report 8798 "Арт-студия"
echo.
echo Игра:         http://127.0.0.1:8080
echo Арт-студия:   http://127.0.0.1:8798
echo Учёт агентов: http://127.0.0.1:8790
echo.
echo Логи: logs\, база - %PG_LOG%, картинки - %COMFY_DIR%\comfy_log.txt
exit /b 0

:the_end
if "%NOPAUSE%"=="1" exit /b 0
echo %cmdcmdline% | findstr /I /C:"/c" >nul 2>&1
if not errorlevel 1 (
  echo.
  echo Нажми любую клавишу, чтобы закрыть это окно. Серверы продолжат работать.
  pause >nul
)
exit /b 0
