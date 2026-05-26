#!/bin/bash
# diagnostic-spotify-api.sh — диагностика для проверки полноты данных от Spotify

USER_ID="${1:-$(docker exec sleepwalkerfm-db-1 psql -U postgres -d sleepwalker -t -c "SELECT user_id FROM spotify_tokens LIMIT 1;" 2>/dev/null | xargs)}"

if [ -z "$USER_ID" ]; then
  echo "❌ User ID не найден. Сначала авторизуйся: http://localhost:8080/auth/spotify/login"
  exit 1
fi

echo "📊 Диагностика Spotify API для: $USER_ID"
echo ""

# Получи access token
ACCESS_TOKEN=$(docker exec sleepwalkerfm-db-1 psql -U postgres -d sleepwalker -t -c "SELECT access_token FROM spotify_tokens WHERE user_id='$USER_ID';" 2>/dev/null | xargs)

if [ -z "$ACCESS_TOKEN" ]; then
  echo "❌ Access token не найден"
  exit 1
fi

echo "🔍 1. Проверяю /me/top/artists с разными time_range параметрами:"
echo ""

for TIME_RANGE in short_term medium_term long_term; do
  echo "⏱️  time_range=$TIME_RANGE:"
  RESPONSE=$(curl -s -H "Authorization: Bearer $ACCESS_TOKEN" \
    "https://api.spotify.com/v1/me/top/artists?time_range=$TIME_RANGE&limit=1")
  
  ARTIST=$(echo "$RESPONSE" | jq -r '.items[0] | "\(.name) | popularity: \(.popularity) | genres: \(.genres | length)"')
  echo "   → $ARTIST"
done

echo ""
echo "⏱️  Info: short_term = последние 4 недели, medium_term = последние 6 месяцев, long_term = всё время"
echo ""

echo "🔍 2. Проверяю токен и его scope:"
docker exec sleepwalkerfm-db-1 psql -U postgres -d sleepwalker -t -c \
  "SELECT 'Scope:' as check_name, scope FROM spotify_tokens WHERE user_id='$USER_ID'
   UNION ALL
   SELECT 'Token expires at:', expires_at::text FROM spotify_tokens WHERE user_id='$USER_ID';"

echo ""
echo "🔍 3. Рекомендации если popularity везде 0:"
echo ""
echo "  ✅ Это может быть нормально для:"
echo "     • Очень новых артистов (< 1000 слушателей)"
echo "     • Локальных/региональных артистов"
echo "     • Если используется time_range=long_term"
echo ""
echo "  🔧 Попробуй:"
echo "     • Свежий рефреш токена: curl http://localhost:8080/auth/spotify/refresh/$USER_ID"
echo "     • Параметр time_range=short_term (может иметь более полные данные)"
echo "     • Другой account с историей прослушиваний"
