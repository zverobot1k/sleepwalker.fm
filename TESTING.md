# Пайплайн тестирования Spotify OAuth + ngrok

## 1. Подготовка переменных окружения

```bash
# Скопируй шаблон
cp .env.example .env

# Заполни значения (BASE_URL и SPOTIFY_REDIRECT_URI обновятся автоматически)
SPOTIFY_CLIENT_ID=<твой-client-id>
DATABASE_URL=postgres://postgres:postgres@localhost:5433/sleepwalker?sslmode=disable
```

## 2. Вариант автоматического обновления URL (рекомендуется)

⚠️ **Проблема**: ngrok генерирует новый URL при каждом старте. Если приложение уже запущено, оно будет использовать старый URL из `.env`.

**Решение**: Используй скрипт `watch-ngrok.sh`, который:
- Запускает ngrok
- Мониторит изменения URL
- Автоматически обновляет `.env`
- Перезапускает приложение

```bash
# Терминал 1: запуск мониторинга
./watch-ngrok.sh

# Скрипт сам:
# 1. Запустит ngrok
# 2. Обновит .env при смене URL
# 3. Перезапустит docker compose
# 4. Будет ждать новые URL и повторять
```

При первом запуске увидишь:
```
✅ Новый ngrok URL обнаружен: https://xxxx-xxx-xxx.ngrok.io
📝 Обновляю .env...
✅ .env обновлен
   BASE_URL=https://xxxx-xxx-xxx.ngrok.io
   SPOTIFY_REDIRECT_URI=https://xxxx-xxx-xxx.ngrok.io/auth/spotify/callback
🔄 Перезапускаю docker compose...
```

---

## 3. Вариант ручного обновления (если не хочешь скрипт)

```bash
# Терминал 1: ngrok
ngrok http 8080

# Терминал 2: мониторь логи приложения
docker compose logs -f app

# Когда ngrok упадет и перестартует:
# 1. Копируешь новый URL из ngrok
# 2. Обновляешь в .env:
#    BASE_URL=https://<новый-url>
#    SPOTIFY_REDIRECT_URI=https://<новый-url>/auth/spotify/callback
# 3. Перезапускаешь приложение:
docker compose restart app
```

---

## 4. Первоначальная настройка Spotify Dashboard (один раз)

Перейди в [Spotify Developer Dashboard](https://developer.spotify.com/dashboard):
1. Выбери своё приложение
2. Нажми **Edit Settings**
3. Добавь несколько Redirect URI вариантов для flexibility:
   ```
   https://localhost:3000/auth/spotify/callback
   http://localhost:8080/auth/spotify/callback
   ```
   (конкретное значение из .env будет переопределено в runtime)
4. Сохрани

## 5. Запуск приложения с Docker Compose

```bash
# Убедись, что .env правильно заполнен
cat .env

# Запусти контейнеры (если используешь скрипт, это уже сделается автоматически)
docker compose up --build

# Проверь логи
docker compose logs -f app
```

Ожидаемые логи:
```
app-1  | [GIN-debug] GET    /health
app-1  | [GIN-debug] GET    /auth/spotify/login
app-1  | 2026/05/24 04:34:04 listening on :8080
db-1   | 2026-05-24 04:34:02.230 UTC [1] LOG:  database system is ready to accept connections
```

## 6. Тестовые запросы

### Проверка здоровья сервера
```bash
curl http://localhost:8080/health
```
Ожидаемый ответ:
```json
{"status":"ok"}
```

### Инициирование OAuth логина
```bash
# Откроешь эту ссылку в браузере (используй из логов скрипта или из .env)
https://$(grep BASE_URL .env | cut -d'=' -f2)/auth/spotify/login
```

**Результат:**
1. Браузер перенаправит тебя на Spotify (запрос разрешений)
2. После одобрения вернётся на `GET /callback?code=...&state=...`
3. Сервер вернёт JSON с профилем пользователя

Ожидаемый ответ:
```json
{
  "user_id": "spotify12345",
  "display_name": "Your Name",
  "email": "your@email.com"
}
```

### Обновление токена
```bash
# Замени <user-id> на полученный user_id
curl -X POST http://localhost:8080/refresh/spotify12345
```

Ожидаемый ответ:
```json
{
  "user_id": "spotify12345",
  "expires_at": "2026-05-24T04:45:00Z",
  "scope": "user-read-email user-read-private ...",
  "token_type": "Bearer"
}
```

## 7. Проверка БД

```bash
# Подключись к PostgreSQL
psql postgresql://postgres:postgres@localhost:5433/sleepwalker

# Проверь таблицы
\dt
```

Ожидаемые таблицы:
- `oauth_states` — состояние PKCE-потока
- `spotify_tokens` — сохранённые токены пользователей

---

## 📋 Полный workflow с watch-ngrok.sh

**Терминал 1: Автоматический мониторинг ngrok**
```bash
./watch-ngrok.sh
```

**Терминал 2: Следи за логами приложения (опционально)**
```bash
docker compose logs -f app
```

**Терминал 3: Тестируй API**
```bash
# Получи URL из первого терминала
BASE_URL="https://xxxx-xxx-xxx.ngrok.io"

# Тест здоровья
curl $BASE_URL/health

# Запусти OAuth flow
open "$BASE_URL/auth/spotify/login"  # macOS
# или
xdg-open "$BASE_URL/auth/spotify/login"  # Linux
# или просто скопируй в браузер
```

---

## 🔄 Что делать, если ngrok перезагрузился

Если используешь `./watch-ngrok.sh`:
- ✅ Скрипт автоматически обнаружит новый URL
- ✅ Обновит `.env`
- ✅ Перезапустит docker compose
- ✅ Напишет новый URL в логи

Просто открой браузер с новым URL из логов скрипта!

## Troubleshooting

### Ошибка: "invalid_redirect_uri"
- ✅ Проверь, что Redirect URI в Spotify Dashboard точно совпадает с одним из перенаправления приложением
- ✅ Убедись, что используешь HTTPS (не HTTP)
- ✅ Если используешь `watch-ngrok.sh`, дождись пока скрипт обновит .env и перезапустит приложение

### Ошибка: "connection refused" на БД
- ✅ Postgres требует время на инициализацию. Подожди 5-10 сек после `docker compose up`
- ✅ Проверь: `docker compose ps`

### Ошибка: "ngrok не запущен" при работе скрипта
- ✅ `./watch-ngrok.sh` сам запускает ngrok, но нужно установить:
  ```bash
  brew install ngrok  # macOS
  apt install ngrok  # Ubuntu/Debian
  ```
- ✅ Или если ngrok уже установлен в другом месте, скрипт всё равно найдёт его

### Как вернуться на ручное управление
Если хочешь выключить `./watch-ngrok.sh`:
```bash
# Ctrl+C в том терминале, где запущен скрипт

# Теперь вручную:
ngrok http 8080
# Копируешь URL
# Обновляешь в .env
# docker compose restart app
```

### ngrok URL меняется слишком часто
- ✅ Используй платную опцию ngrok (Reserved Domains) для статического URL
- ✅ Или оставляй ngrok запущённым в одном окне (не рестартуй)

## Wrapped-статистика: полезные endpoints

После получения access token, через OAuth сервер можно вызвать ручки Spotify API для:

```
GET /me/top/{type}  — топ треки/артисты (time_range: short_term, medium_term, long_term)
GET /me/recently-played  — недавно прослушанные
GET /me/playlists  — плейлисты пользователя
GET /me/personalization/top/tracks  — музыкальные предпочтения
```

Подробнее в [openapi.yaml](./openapi.yaml) на путях `/me/top/{type}` и `/me/player/recently-played`.
