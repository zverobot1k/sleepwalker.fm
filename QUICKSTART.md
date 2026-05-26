# 🚀 Быстрый старт (5 минут)

## Шаг 1: Clonируй репозиторий (если еще не сделал)
```bash
cd sleepwalker.fm
```

## Шаг 2: Подготовь .env
```bash
cp .env.example .env
# Заполни только SPOTIFY_CLIENT_ID в .env
# Остальное будет заполнено скриптом автоматически
```

## Шаг 3: Запусти автоматический скрипт (рекомендуется)

```bash
./watch-ngrok.sh
```

Скрипт сам:
- 🔄 Запустит ngrok
- 📝 Обновит BASE_URL и SPOTIFY_REDIRECT_URI в .env
- 🐳 Запустит docker compose
- 🔍 Будет следить за изменениями URL и перезапускать приложение

**Вывод скрипта:**
```
✅ Новый ngrok URL обнаружен: https://xxxx-xxx-xxx.ngrok.io
📝 Обновляю .env...
✅ .env обновлен
   BASE_URL=https://xxxx-xxx-xxx.ngrok.io
   SPOTIFY_REDIRECT_URI=https://xxxx-xxx-xxx.ngrok.io/auth/spotify/callback
🔄 Перезапускаю docker compose...
✅ Приложение готово на https://xxxx-xxx-xxx.ngrok.io
```

## Шаг 4: Обнови Spotify Dashboard (один раз)

1. Перейди в [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)
2. Выбери приложение → **Edit Settings**
3. Добавь Redirect URI (любой вариант для flexibility):
   ```
   https://localhost:8080/auth/spotify/callback
   ```
   (конкретный URL будет в логах скрипта)
4. Сохрани

## Шаг 5: Тестируй!

Когда скрипт выведет URL:
```bash
# Открой в браузере
https://xxxx-xxx-xxx.ngrok.io/auth/spotify/login
```

Или в терминале:
```bash
# Скопируй URL из логов скрипта и используй вместо xxxx-xxx-xxx
curl https://xxxx-xxx-xxx.ngrok.io/health
```

---

## 📋 Альтернатива: Ручное управление

Если не хочешь скрипт, делай вручную:

```bash
# Терминал 1: ngrok
ngrok http 8080

# Код URL из вывода (например: https://abcd-1234.ngrok.io)

# Терминал 2: обнови .env
BASE_URL=https://abcd-1234.ngrok.io
SPOTIFY_REDIRECT_URI=https://abcd-1234.ngrok.io/auth/spotify/callback

# Терминал 3: запусти приложение
docker compose up --build
```

---

## 🆘 Проблемы?

| Проблема | Решение |
|----------|---------|
| "invalid_redirect_uri" | Синхронизируй URL в Spotify Dashboard с логами скрипта |
| "connection refused" | Подожди 5-10 сек после старта docker compose |
| ngrok не запущен | `brew install ngrok` (или используй ручный вариант) |
| Нужен статический URL | Позови платный ngrok Premium (Reserved Domains) |

Полная документация: [TESTING.md](./TESTING.md)
