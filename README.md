# Sleepwalker.fm — Spotify Wrapped Dashboard

Приложение для сбора и анализа Spotify статистики пользователя. OAuth 2.0 + PKCE, PostgreSQL, Gin, Docker.

## Архитектура

```
┌─────────────────────────────────────────────────────────────┐
│                    Internal Layers                          │
├─────────────────────────────────────────────────────────────┤
│ Transport (HTTP)                                            │
│  ├─ /health (health check)                                 │
│  ├─ /auth/spotify/login (инициирование OAuth)             │
│  ├─ /auth/spotify/callback (обработка возврата)           │
│  └─ /refresh/:userId (обновление токена)                  │
├─────────────────────────────────────────────────────────────┤
│ Service (Business logic)                                    │
│  └─ OAuthService: PKCE flow, token exchange, refresh       │
├─────────────────────────────────────────────────────────────┤
│ Repository (Data Access)                                    │
│  ├─ OAuthStateRepo: хранение state + verifier              │
│  └─ TokenRepo: сохранение access/refresh tokens            │
├─────────────────────────────────────────────────────────────┤
│ Config (Environment loading)                                │
│  └─ Загрузка переменных: DATABASE_URL, SPOTIFY_CLIENT_ID   │
└─────────────────────────────────────────────────────────────┘
                          ↓
              PostgreSQL (docker-compose)
```

## Быстрый старт

**⏱️ 5 минут на старт**: Смотри [QUICKSTART.md](./QUICKSTART.md)

```bash
# 1. Скопируй конфиг
cp .env.example .env
# ^ Заполни SPOTIFY_CLIENT_ID

# 2. Запусти автоскрипт (обновляет .env при смене ngrok URL)
./watch-ngrok.sh

# 3. Открой браузер на выведенный URL
https://xxxx-xxx-xxx.ngrok.io/auth/spotify/login
```

---

## Требования

- **Go**: 1.23+
- **Docker & Docker Compose**
- **Spotify Developer Account** (https://developer.spotify.com)
- **ngrok** (для локального HTTPS туннеля): `brew install ngrok`

## Требования

- **Go**: 1.23+
- **Docker & Docker Compose**
- **Spotify Developer Account** (https://developer.spotify.com)
- **ngrok** (для локального HTTPS туннеля): `brew install ngrok`

## Быстрый старт (5 минут)

👉 **Полная инструкция**: [QUICKSTART.md](./QUICKSTART.md)

TL;DR:
```bash
cp .env.example .env
# ↑ Заполни SPOTIFY_CLIENT_ID

./watch-ngrok.sh
# ↑ Скрипт запустит ngrok, obsessionвит .env, запустит приложение

# Следи за логами скрипта для URL
```

## API Endpoints

### Health Check
```bash
GET /health
```
```json
{"status":"ok"}
```

### OAuth Login
```bash
GET /auth/spotify/login
```
Перенаправляет на Spotify для авторизации.

### OAuth Callback
```bash
GET /auth/spotify/callback?code=...&state=...
```
Обработка возврата от Spotify. Возвращает:
```json
{
  "user_id": "spotify_user_id",
  "display_name": "User Name",
  "email": "user@email.com"
}
```

### Token Refresh
```bash
POST /refresh/:userId
```
Обновляет access token. Возвращает:
```json
{
  "user_id": "spotify_user_id",
  "expires_at": "2026-05-24T05:00:00Z",
  "scope": "user-read-email user-read-private ...",
  "token_type": "Bearer"
}
```

## База данных

PostgreSQL 16. Автоматически создаёт таблицы при старте:

- `oauth_states` — state + verifier + expiry (для PKCE)
- `spotify_tokens` — user_id, access/refresh tokens, expiry

Подключение:
```bash
psql postgresql://postgres:postgres@localhost:5433/sleepwalker
```

Порты:
- Хост: `5433`
- Контейнер: `5432`

## Тестирование

Полный пайплайн в [TESTING.md](./TESTING.md).

Quickstart:
```bash
# 1. Запусти ngrok
ngrok http 8080

# 2. Обнови .env с новым URL

# 3. Запусти контейнеры
docker compose up --build

# 4. Открой в браузере
https://xxxx-xxx-xxx.ngrok.io/auth/spotify/login
```

## Wrapped-статистика

Полезные Spotify API endpoints в [WRAPPED_ENDPOINTS.md](./WRAPPED_ENDPOINTS.md).

Ключевые ручки:
- `GET /me/top/tracks` — топ треки
- `GET /me/top/artists` — топ артисты
- `GET /audio-features` — музыкальный профиль
- `GET /me/player/recently-played` — история прослушивания

## Структура проекта

```
sleepwalker.fm/
├─ internal/
│  ├─ main.go                    (entry point)
│  ├─ config/
│  │  └─ config.go              (env loading)
│  ├─ domain/
│  │  └─ models.go              (entities: OAuthState, SpotifyTokens)
│  ├─ service/
│  │  └─ spotify/
│  │     └─ oauth_service.go    (PKCE flow logic)
│  ├─ repository/
│  │  └─ postgres/
│  │     ├─ db.go               (GORM init)
│  │     ├─ models.go           (GORM models)
│  │     ├─ oauth_state_repo.go (state CRUD)
│  │     ├─ token_repo.go       (token CRUD)
│  │     └─ migrations/
│  │        └─ 001_create_tables.sql
│  └─ transport/
│     └─ http/
│        ├─ router.go           (Gin routes)
│        └─ handlers/
│           ├─ oauth_handler.go
│           └─ health.go
├─ Dockerfile                    (Go 1.23-alpine)
├─ docker-compose.yml            (App + Postgres)
├─ go.mod / go.sum              (dependencies)
├─ .env.example                 (template)
├─ TESTING.md                   (test pipeline)
├─ WRAPPED_ENDPOINTS.md         (Spotify API guide)
└─ README.md                    (этот файл)
```

## Deploy

Docker образ готов к deployment:
- **Base image**: `golang:1.23-alpine` (build) → `alpine:3.20` (runtime)
- **Binary**: `/app/app`
- **Port**: `8080`
- **Environment**: все из `.env`

Пример Kubernetes deployment или CloudRun — добавить в будущем.

## Решение проблем

### Ошибка: "invalid_redirect_uri"
- ✅ Проверь, что URL в Spotify Dashboard точно совпадает с `SPOTIFY_REDIRECT_URI` в коде

### Ошибка: "connection refused" на PostgreSQL
- ✅ Postgres требует 5-10 сек на инициализацию. Подожди перед первым запросом

### Ngrok URL меняется
- ✅ Обнови `.env` и перезагрузи приложение

## Лицензия

MIT

## Автор

[Maksim Blokhin](https://github.com/maksimblohin)
