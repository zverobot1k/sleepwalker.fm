# 🎵 Тестирование endpoint топ артистов

## 📋 Что мы реализовали

```
GET /api/spotify/top/artists/:userId
```

Этот endpoint:
1. Получает `userId` из URL параметра
2. Достаёт `access_token` пользователя из БД
3. Вызывает Spotify API `/me/top/artists`
4. Возвращает топ артистов пользователя

---

## 🧪 Шаг 1: Получить user_id (из OAuth)

Когда ты авторизовался через `/auth/spotify/login`, получил ответ:

```json
{
  "user_id": "spotify12345",
  "display_name": "Maksim",
  "email": "you@email.com"
}
```

**Вот это `user_id` нам нужен для тестирования!**

Если ты не сохранил, можешь:
- Посмотреть в логах браузера (Network tab)
- Или повторно авторизоваться, чтобы получить `user_id`
- Или посмотреть в БД:
  ```bash
  psql postgresql://postgres:postgres@localhost:5433/sleepwalker
  SELECT user_id FROM spotify_tokens LIMIT 1;
  ```

---

## 🧪 Шаг 2: Вызови новый endpoint

### Вариант 1: curl (командная строка)

```bash
# Замени <user-id> на свой user_id
curl -X GET "http://localhost:8080/api/spotify/top/artists/<user-id>"
```

**Пример с реальным user_id:**
```bash
curl -X GET "http://localhost:8080/api/spotify/top/artists/spotify12345"
```

**Ожидаемый ответ:**
```json
{
  "items": [
    {
      "id": "3TVXtAsB1e9WnuMCVS3xKP",
      "name": "Nirvana",
      "popularity": 87,
      "genres": ["grunge", "rock"],
      "images": [
        {
          "height": 640,
          "width": 640,
          "url": "https://i.scdn.co/image/..."
        }
      ],
      "external_urls": {
        "spotify": "https://open.spotify.com/artist/3TVXtAsB1e9WnuMCVS3xKP"
      }
    },
    ...
  ],
  "total": 365,
  "limit": 20
}
```

### Вариант 2: Postman

1. Создай новый request: `GET`
2. URL: `http://localhost:8080/api/spotify/top/artists/spotify12345`
3. Нажми **Send**

### Вариант 3: Браузер

```
http://localhost:8080/api/spotify/top/artists/spotify12345
```

---

## 📊 Что возвращает API

| Поле | Тип | Описание |
|------|-----|---------|
| `items` | Array | Массив артистов (по умолчанию 20) |
| `items[].id` | String | Spotify ID артиста |
| `items[].name` | String | Имя артиста |
| `items[].popularity` | Number | Популярность 0-100 |
| `items[].genres` | Array | Жанры артиста |
| `items[].images` | Array | Картинки артиста |
| `total` | Number | Всего артистов у пользователя |
| `limit` | Number | Вернулось (в запросе: 20) |

---

## 🔧 Параметры запроса (future enhancement)

Сейчас хардкодированы:
```go
url := "https://api.spotify.com/v1/me/top/artists" +
       "?time_range=long_term" +  // long_term | medium_term | short_term
       "&limit=20"                // 1-50
```

Можно добавить query параметры:
```bash
# Топ артистов за последние 4 недели (вместо всего времени)
http://localhost:8080/api/spotify/top/artists/spotify12345?time_range=short_term

# Получить топ 50 вместо 20
http://localhost:8080/api/spotify/top/artists/spotify12345?limit=50
```

---

## 🐛 Troubleshooting

### Ошибка: "user tokens not found"
```
{"error":"user tokens not found"}
```
**Решение:** 
- Проверь, что user_id правильный (из ответа OAuth)
- Может быть, ты не авторизовался ещё
- Проверь БД: `SELECT user_id FROM spotify_tokens;`

### Ошибка: "no access token"
```
{"error":"no access token"}
```
**Решение:**
- Токен может быть истёк, нужно обновить через `/refresh/:userId`

### Ошибка: "spotify api error: 401"
```
{"error":"spotify api error: 401 - ..."}
```
**Решение:**
- Access token истёк, обновим его:
  ```bash
  curl -X POST http://localhost:8080/refresh/<user-id>
  ```
- Затем попробой запрос снова

### Ошибка: "connection refused"
**Решение:** Дождись загрузки контейнеров (5-10 сек) и проверь:
```bash
docker compose ps
docker compose logs app
```

---

## 📝 Полный workflow тестирования

```bash
# 1. Авторизуйся (если еще не авторизован)
open "https://$(grep BASE_URL .env | cut -d'=' -f2)/auth/spotify/login"
# или в браузер: https://roundness-ecosystem-sitting.ngrok-free.dev/auth/spotify/login

# 2. Скопируй user_id из ответа

# 3. Вызови новый endpoint
USER_ID="spotify12345"  # замени на свой
curl http://localhost:8080/api/spotify/top/artists/$USER_ID | jq

# 4. Посмотри результат (красиво):
curl http://localhost:8080/api/spotify/top/artists/$USER_ID | jq '.items[0:5]'
# ^ покажет топ 5 артистов
```

---

## 🎯 Next steps (что добавить дальше)

1. **Query параметры:**
   ```go
   limit := c.DefaultQuery("limit", "20")
   timeRange := c.DefaultQuery("time_range", "long_term")
   ```

2. **Топ треков:**
   ```
   GET /api/spotify/top/tracks/:userId
   ```

3. **Audio features (музыкальный профиль):**
   ```
   GET /api/spotify/audio-features/:userId
   ```

4. **История слушания:**
   ```
   GET /api/spotify/recently-played/:userId
   ```

---

## 💡 Как это работает под капотом

```
Клиент (браузер)
    ↓
curl http://localhost:8080/api/spotify/top/artists/spotify12345
    ↓
Router (получает request)
    ↓
SpotifyAPIHandler.GetTopArtists()
    ├─ Достаёт user_id из URL
    ├─ Запрашивает access_token из БД
    ├─ Вызывает fetchTopArtistsFromSpotify()
    │   ├─ Создаёт HTTP GET запрос
    │   ├─ Добавляет Authorization: Bearer <token>
    │   └─ Вызывает https://api.spotify.com/v1/me/top/artists
    ├─ Парсит JSON ответ от Spotify
    └─ Возвращает клиенту
    ↓
JSON ответ с артистами
```

---

Готово! 🚀 Протестируй прямо сейчас!
