# Spotify API endpoints для Wrapped-статистики

## Основные ручки для сбора данных

### 1. **Топ треков и артистов**
```
GET /me/top/{type}
```
- `type`: `tracks` или `artists`
- Query параметры:
  - `time_range`: `long_term` (несколько лет) | `medium_term` (6 месяцев) | `short_term` (4 недели)
  - `limit`: 1-50 (по умолчанию 20)
  - `offset`: 0-xxxxx для пагинации

**Exemplo:**
```bash
GET /me/top/tracks?time_range=long_term&limit=50
```

**Возвращает:**
- Tracks с популярностью, жанром, альбомом
- Order: сортировано по "affinity" (как часто слушаешь)

---

### 2. **Недавно прослушанные**
```
GET /me/player/recently-played
```
- Query параметры:
  - `limit`: max 50 (default 20)
  - `before`: Unix timestamp (миллисекунды)
  - `after`: Unix timestamp для cursor-based pagination

**Возвращает:**
- PlayHistoryObject: track + played_at + context (плейлист/альбом, откуда был запущен)
- Один трек может быть несколько раз (если слушал несколько раз)

---

### 3. **Плейлисты пользователя**
```
GET /me/playlists
```
- Query параметры:
  - `limit`: 1-50 (default 20)
  - `offset`: для пагинации

**Возвращает:**
- Список плейлистов с именем, картинкой, кол-во треков
- Полезно для контекстуализации слушань

---

### 4. **Аудиофичи (audio features)**
```
GET /audio-features/{id}
GET /audio-features?ids=id1,id2,id3  (max 100 ids)
```

**Возвращает:**
- `acousticness`: 0.0-1.0 (насколько "акустичный")
- `danceability`: 0.0-1.0 (пригодность для танцев)
- `energy`: 0.0-1.0 (интенсивность)
- `instrumentalness`: 0.0-1.0 (отсутствие вокала)
- `key`: -1 to 11 (musical key)
- `liveness`: 0.0-1.0 (живая запись vs студия)
- `loudness`: -60 to 0 dB
- `mode`: 0 (minor) | 1 (major)
- `speechiness`: 0.0-1.0 (наличие разговорной речи)
- `tempo`: BPM
- `time_signature`: 3-7 (размер: 3/4, 4/4 и т.д.)
- `valence`: 0.0-1.0 (музыкальный позитивизм)

**Полезно для:** анализа музыкального "профиля" за период

---

### 5. **Информация о треке / альбоме**
```
GET /tracks/{id}
GET /albums/{id}
GET /artists/{id}
```

**Возвращает:**
- Полная информация: жанры, изображения, популярность, дата выхода
- Полезно для обогащения данных из top/recently-played

---

## Архитектура сбора данных для Wrapped

```
┌─────────────────────────────────┐
│ User OAuth (access_token)       │
└────────────┬────────────────────┘
             │
             ├──> GET /me/top/tracks?time_range=long_term
             │    └─> Список TOP 10-50 треков
             │
             ├──> GET /me/top/artists?time_range=long_term
             │    └─> Список TOP 10-50 артистов
             │
             ├──> GET /me/player/recently-played?limit=50
             │    └─> Последние 50 воспроизведений
             │
             ├──> GET /audio-features?ids=track1,track2,...
             │    └─> Музыкальный профиль треков
             │
             └──> AGGREGATE & ANALYZE
                  - Жанры (из Artist)
                  - Энергия (из audio-features)
                  - Танцевальность
                  - Кол-во часов (по recently-played + duration)
                  - Новые явления / открытия
```

---

## Query примеры для простого Wrapped

### Сценарий 1: "Top 10 tracks of the year"
```bash
curl -H "Authorization: Bearer $ACCESS_TOKEN" \
  "https://api.spotify.com/v1/me/top/tracks?time_range=long_term&limit=10"
```

### Сценарий 2: "Music profile stats"
```bash
# 1. Получи TOP 50 треков
curl -H "Authorization: Bearer $ACCESS_TOKEN" \
  "https://api.spotify.com/v1/me/top/tracks?limit=50&time_range=long_term"

# 2. Передай их IDs в audio-features (batch)
curl -H "Authorization: Bearer $ACCESS_TOKEN" \
  "https://api.spotify.com/v1/audio-features?ids=id1,id2,id3,...,id50"

# 3. Aggregated stats:
#    - Average valence (mood)
#    - Average energy
#    - Average danceability
#    - Median tempo
```

### Сценарий 3: "Listening hours estimate"
```bash
# 1. Получи recently-played с before/after для периода
curl -H "Authorization: Bearer $ACCESS_TOKEN" \
  "https://api.spotify.com/v1/me/player/recently-played?limit=50&before=$TIMESTAMP_END&after=$TIMESTAMP_START"

# 2. Пройди по всем с pagination (берёшь next URL)

# 3. Для каждого track получи duration из GET /tracks/{id}

# 4. Сумма всех duration_ms = total listening time
```

---

## Rate Limits & Best Practices

- **Rate limit**: ~450 requests per 15 minutes (per token)
- **Batch endpoints**: Используй `ids` параметр вместо отдельных запросов
  - `GET /audio-features?ids=id1,id2,...` (до 100 IDs за раз)
  - `GET /albums?ids=id1,id2,...` (до 20 IDs за раз)
- **Pagination**: Используй `offset` или cursor если много данных
- **Кэширование**: Spotify данные не меняются часто, можно кэшировать в БД

---

## Дополнительные ручки (опционально)

- `GET /me/library/contains` — проверить, сохранены ли в "Your Music"
- `GET /me/personalization/top/artists` — персонализованная статистика
- `GET /recommendations` — рекомендации на основе топ треков
- `GET /search?q={query}&type=artist,track` — поиск (например, для debug)

---

## Вывод

Для **простого Wrapped MVP**:

1. ✅ `/me/top/tracks` (long_term только)
2. ✅ `/me/top/artists` (long_term)
3. ✅ `/audio-features` (batch для инсайтов)
4. ✅ `/me/recently-played` (для кол-ва часов прослушивания)

**Кол-во запросов:** ~4-5 на сессию = хорошо в рамках rate limits.

Для **хардкорного Wrapped**:
- Multi-period analysis (short/medium/long_term)
- Жанровый анализ
- Рекомендации (seed artists/tracks)
- Сравнение с past years
