# Sleepwalker.fm — Текущее состояние проекта (snapshot v2)

## Краткий обзор
Проект состоит из двух основных частей:
- Backend (Go): сбор данных Spotify, обогащение данными Last.fm, кэширование (Redis), рекомендации, HTTP API.
- Frontend (Next.js, React, TypeScript): дашборд пользователя, рендер-first подход, запросы к backend API.

Цель недавнего рефактора: превратить `Snapshot` в быстрый UI-объект (core snapshot), вынести тяжёлое обогащение в асинхронные процессы, сделать Last.fm основным источником seed-данных для рекомендаций, и сделать фронтенд отзывчивым (render-first).

## Архитектура и ключевые компоненты

- internal/service/pipeline — строит `SpotifySnapshot`, кэширует его в Redis, использует `singleflight` чтобы избегать параллельных rebuild'ов.
  - `SpotifySnapshot` (snapshot v2): `user_id`, `time_range`, `top_tracks`, `top_artists`, `recently_played`, `audio_features` (опционально), `created_at`, плюс async-поля `top_genres`, `seed_*`.
  - `FetchSnapshot(ctx,userID,timeRange)` — читает кэш, при miss вызывает `buildSnapshot` и пишет в кэш.
  - `buildSnapshot` — сейчас собирает только core: `top_tracks`, `top_artists`, `recently_played`; тяжёлое обогащение (audio_features, genre agg, seed selection) — асинхронно.

- internal/service/spotify — обёртка для Spotify API с поведением backoff и `emptySpotifyPayload()` при DEGRADED/forbidden. Используется и в handler-ах и в pipeline.

- internal/service/recommendation — движок рекомендаций. Сейчас логика: Last.fm-driven seeds (если есть), затем Spotify fallback, с попытками добрать до нужного лимита.

- internal/transport/http/handlers — Gin handlers:
  - `GET /api/spotify/snapshot/:userId` — быстрый snapshot endpoint, возвращает cached snapshot; при пустом `recently_played` есть fallback: прямой fetch recently-played через user access token; затем запускается GenreStats и другие enrichment-методы (иногда асинхронно).
  - `GET /api/recommendations/:userId` — может быть тяжелым (Last.fm graph, расширение кандидатов).
  - `GET /api/wrapped/timeline/:userId`, `GET /api/stats/listening-time/:userId`, и др.

- Frontend: `sleepwalker.fm.front/src/app/(protected)/dashboard/page.tsx` — dashboard компонент.
  - Вызовы: `api.snapshot(userId)` и `api.recommendations(userId)`.
  - Были изменения на render-first, но текущая версия содержит `Promise.allSettled([snapshot, recommendations])` в одном useEffect, что блокирует готовность UI до завершения recommendations.

## Что сейчас возвращает snapshot API (пример)
Пример реального ответа (user `spotify_3aa381624f9909c9ba9aa72fd0c1f3e7`):

- Полный ответ сохранён в `/tmp/snapshot_response.json` при локальных тестах.
- Сокращённый факт ответа:
  - `snapshot.top_tracks` — 50 элементов
  - `snapshot.top_artists` — 50 элементов
  - `snapshot.recently_played` — [] (пусто)
  - `snapshot.audio_features` — отсутствует / null
  - `top_genres` — массив жанров (агрегация Last.fm)

Логи подтверждают: при cache-miss pipeline делает последовательные Spotify DoGET к `me/top/tracks`, `me/top/artists`, `me/player/recently-played`, затем пишет snapshot в кэш.

## Логирование этапов buildSnapshot (как смотреть)
В логах backend видно DIAG-сообщения (примерные форматы):

- `DIAG snapshot cache MISS user=<user> time_range=<range>`
- `DIAG spotify DoGET start: user=<user> url=https://api.spotify.com/v1/me/top/tracks?limit=50&time_range=...`
- `DIAG spotify DoGET start: user=<user> url=https://api.spotify.com/v1/me/top/artists?limit=50&time_range=...`
- `DIAG spotify DoGET start: user=<user> url=https://api.spotify.com/v1/me/player/recently-played?limit=50`
- `DIAG snapshot cache REBUILT user=...`

На основе live-ответа и кэша текущие счётчики после buildSnapshot:

SNAPSHOT DEBUG:
- top_tracks=50
- top_artists=50
- recently_played=0
- audio_features=0

(эти значения соответствуют данным в Redis и JSON ответа)

## Что попадает в кэш
- Ключ snapshot: `swfm:snapshot:<userId>:<time_range>` — содержит объект `SpotifySnapshot` с полями как в ответе.
- Ответ handler'а также кэшируется как `swfm:response:["snapshot","<userId>","<time_range>"]`.
- Проверка Redis показывает совпадающие размеры массивов: `top_tracks=50`, `top_artists=50`, `recently_played=0`.

## Что фронтенд реально использует vs. что приходит
- Приходящие поля: `snapshot.top_tracks`, `snapshot.top_artists`, `snapshot.recently_played` (пустой), `top_genres` (присутствует), `audio_features` отсутствует.
- Dashboard использует:
  - `top_tracks`, `top_artists` — показываются
  - `recently_played` — используется для `timeline` и `listening stats` (теперь пусто → пустой timeline, listening_time = 0)
  - `audio_features` — используется для панели AudioFeatures (пустая)
  - `top_genres` — используется и отображается

## Причины длительной загрузки dashboard (список)
1. Promise.allSettled([snapshot, recommendations]) в верхнем useEffect: recommendations выполняется долго, поэтому UI ждёт завершения. (см. `sleepwalker.fm.front/src/app/(protected)/dashboard/page.tsx`)
2. `GetRecommendations` на backend может занимать очень много времени (Last.fm graph expansion, сеть) — подтверждено логами (`GET /api/recommendations/...` занимал минуты в рядах случаев).
3. Наличие вложённого `useEffect` внутри `useMemo` в `page.tsx` — нарушение правил хуков и потенциальный источник бессмысленных re-render/гидрационных проблем.
4. `recently_played` пуст — не блокирует рендер, но делает timeline и listening stats пустыми.
5. singleflight и кэш работают корректно, но первый запрос (cache miss) может выглядет медленным при полном rebuild (но не дольше, чем задержка от recommendations).

## Локальный запуск и проверка (коротко)
Запуск в dev (docker-compose):

```bash
docker compose up --build
# backend слушает на :8080, frontend на :3000, redis на :6379, postgres на :5433
```

Проверки:
- Snapshot (пример):
```bash
curl "http://localhost:8080/api/spotify/snapshot/<userId>?time_range=medium_term" -v
```
- Redis keys:
```bash
docker compose exec -T redis redis-cli keys "swfm:*"
docker compose exec -T redis redis-cli GET "swfm:snapshot:<userId>:medium_term"
```
- Логи backend:
```bash
docker compose logs backend --since 10m
```

## Отладка и рекомендации (не вношу правок без вашего разрешения)
- Немедленно заметные причины проблемы UX:
  - Сделать frontend независимым от `recommendations` на initial render: загружать `snapshot` первым и отображать UI, а `recommendations` грузить в фоне и обновлять часть экрана по приходу данных.
  - Убрать/перенести `useEffect` изнутри `useMemo` — соблюсти правила хуков.
  - В backend: оставить fallback recently_played (он уже добавлен) и, если Spotify часто возвращает `emptySpotifyPayload()`, добавить логирование и ограниченный retry/метрики для понимания частоты.
- Дополнительно (по желанию): добавить метрики времени выполнения `recommendations` и лимиты/тайм-ауты в recommendation engine, чтобы не блокировать UX.

## Что я уже собрал
- Live JSON snapshot (пример) сохранён в `/tmp/snapshot_response.json` локально при тесте.
- Логи backend (включая DIAG строки) были прочитаны от docker compose; из них взяты ключевые наблюдения о DoGET и длительных вызовах recommendations.

---

Если нужно, подготовлю:
- A) Полный текст лога `recommendations` (включая времена и stack traces) — чтобы понять узкие места внутри recommendation engine.
- B) Набор мелких PR-патчей (frontend и backend) для незамедлительного UX-улучшения (рендер-first, убрать Promise.allSettled блокировку, исправить hooks). (Не буду применять без подтверждения.)

Что делать дальше: A (показать полные логи recommendations) или B (предложить конкретные изменения/PR)?