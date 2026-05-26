#!/bin/bash
# test-top-artists.sh — удобное тестирование топ артистов

BASE_URL="http://localhost:8080"

if [ -z "$1" ]; then
  echo "❌ Используй: $0 <user-id>"
  echo ""
  echo "Где взять user-id:"
  echo "  1. Авторизуйся: $BASE_URL/auth/spotify/login"
  echo "  2. Скопируй user_id из ответа"
  echo ""
  echo "Или из БД:"
  echo "  psql postgresql://postgres:postgres@localhost:5433/sleepwalker"
  echo "  SELECT user_id FROM spotify_tokens LIMIT 1;"
  echo ""
  echo "Пример использования:"
  echo "  $0 spotify12345"
  exit 1
fi

USER_ID=$1

echo "🎵 Получаю топ артистов для пользователя: $USER_ID"
echo ""

# Вызов API
RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$BASE_URL/api/spotify/top/artists/$USER_ID")
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | head -n-1)

# Проверка статуса
if [ "$HTTP_CODE" -ne 200 ]; then
  echo "❌ Ошибка (HTTP $HTTP_CODE):"
  echo "$BODY" | jq . 2>/dev/null || echo "$BODY"
  exit 1
fi

echo "✅ Успешно! (HTTP 200)"
echo ""

# Красивый вывод
echo "📊 Результаты:"
echo "$BODY" | jq '{
  total: .total,
  limit: .limit,
  top_5: .items[0:5] | map({
    name: .name,
    popularity: .popularity,
    genres: .genres[0:2],
    spotify_url: .external_urls.spotify
  })
}' || echo "$BODY" | jq '.'

echo ""
echo "🎶 Все артисты (только имена):"
echo "$BODY" | jq -r '.items[] | "  • \(.name) (популярность: \(.popularity))"'

echo ""
echo "💡 Вот несколько команд для экспериментов:"
echo ""
echo "1. Топ 50 артистов:"
echo "   curl 'http://localhost:8080/api/spotify/top/artists/$USER_ID?limit=50' | jq '.total'"
echo ""
echo "2. Только имена артистов:"
echo "   curl -s 'http://localhost:8080/api/spotify/top/artists/$USER_ID' | jq -r '.items[].name'"
echo ""
echo "3. Артисты и их жанры:"
echo "   curl -s 'http://localhost:8080/api/spotify/top/artists/$USER_ID' | jq '.items[] | {name, genres}'"
