#!/bin/bash
# get-user-id.sh — получить user_id из базы данных

echo "🔍 Получаю список пользователей..."
echo ""

USERS=$(docker exec sleepwalkerfm-db-1 psql -U postgres -d sleepwalker -t -c "SELECT user_id FROM spotify_tokens LIMIT 1;" 2>/dev/null)

if [ -z "$USERS" ]; then
  echo "❌ Не найдены пользователи в БД"
  echo ""
  echo "Убедись что:"
  echo "  1. Docker контейнеры запущены: docker compose ps"
  echo "  2. Ты авторизовался через Spotify OAuth"
  echo ""
  echo "Для авторизации:"
  echo "  http://localhost:8080/auth/spotify/login"
  exit 1
fi

echo "✅ Найден пользователь:"
echo ""
echo "   User ID: $USERS"
echo ""
echo "Используй для тестирования:"
echo ""
echo "   ./test-top-artists.sh $USERS"
echo ""
echo "Или напрямую:"
echo ""
echo "   curl http://localhost:8080/api/spotify/top/artists/$USERS | jq"
