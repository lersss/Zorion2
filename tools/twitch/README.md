# tools/twitch — управление эфиром Twitch одной командой

Служебный инструмент для создателя (не часть игры). Меняет **заголовок, категорию,
теги и язык** эфира Zorion через официальный Twitch API, вместо ручного набора в
дашборде.

Работает на Windows PowerShell 5.1. Секреты и токены хранятся **вне репозитория**,
в `%USERPROFILE%\.zorion-twitch\`. В репозиторий они не попадают никогда.

## Разовая настройка

### 1. Создать приложение в Twitch dev-консоли

1. Открой <https://dev.twitch.tv/console/apps> → **Register Your Application**.
2. **Name** — любое (например, `Zorion Stream Setup`).
3. **OAuth Redirect URL** — ровно:

   ```
   http://localhost:3000
   ```

   Важно: именно `localhost`, без завершающего слэша. Скрипт слушает порт 3000 на
   `127.0.0.1`, а в запросе на авторизацию отправляет `http://localhost:3000` —
   так адрес совпадает с зарегистрированным.
4. **Category** — любая (например, `Application Integration`).
5. Нажми **Create**, затем **Manage** → скопируй **Client ID** и сгенерируй
   **Client Secret**.

### 2. Первая авторизация

```powershell
powershell -File tools/twitch/set-stream.ps1 -Login
```

Скрипт спросит `Client ID` и `Client Secret` (секрет вводится скрытно), сохранит их
в `%USERPROFILE%\.zorion-twitch\config.json`, откроет браузер и дождётся редиректа.
После подтверждения токен сохранится в `%USERPROFILE%\.zorion-twitch\token.json`.

Достаточно сделать это **один раз**.

## Если PowerShell не даёт запустить скрипт

Если видишь ошибку вида «выполнение сценариев отключено в этой системе»
(Execution Policy), добавь `-ExecutionPolicy Bypass`:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/twitch/set-stream.ps1 -Login
```

## Примеры команд

Во всех примерах ниже при блокировке политики добавляй `-ExecutionPolicy Bypass`
после `-NoProfile`.

```powershell
# показать текущие заголовок, категорию, теги, язык
powershell -File tools/twitch/set-stream.ps1 -Get

# поменять всё разом
powershell -File tools/twitch/set-stream.ps1 `
    -Title "Разработка Zorion: генерация планет" `
    -Category "Software and Game Development" `
    -Tags "gamedev,indiedev,space" `
    -Language ru

# поменять только заголовок (остальное не трогается)
powershell -File tools/twitch/set-stream.ps1 -Title "Стрим Zorion"

# посмотреть, что будет отправлено, ничего не отправляя
powershell -File tools/twitch/set-stream.ps1 -Title "Тест" -Category "Science & Technology" -DryRun
```

- `-Category` — **точное** имя игры/категории на Twitch (как в поиске). Если не
  найдено, скрипт скажет об этом и ничего не изменит.
- `-Tags` — через запятую, не более 10 штук. Регистр приводится к нижнему.
- Незаданные параметры **не затираются**: отправляются только заданные поля.

## Где лежат секреты

| Файл | Что внутри |
|---|---|
| `%USERPROFILE%\.zorion-twitch\config.json` | `client_id`, `client_secret` |
| `%USERPROFILE%\.zorion-twitch\token.json` | `access_token`, `refresh_token`, срок действия, scope |

Файлы создаются и обновляются автоматически. Не копируй их в репозиторий и не
публикуй. Скрипт их не логирует.

## Если «токен истёк»

Обычно ничего делать не нужно: при истечении access token скрипт сам обновит его
по refresh token и перезапишет `token.json`.

Если обновление не прошло (например, доступ отозвали в Twitch) — скрипт скажет
запустить `-Login` заново, и авторизация выдаст свежий токен.

## Ограничения

- Нужен один раз выданный scope `channel:manage:broadcast`.
- Порт 3000 должен быть свободен во время `-Login`.
- Язык задаётся кодом ISO 639-1 (`ru`, `en`, …).
