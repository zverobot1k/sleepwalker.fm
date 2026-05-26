#!/bin/bash
# watch-ngrok.sh — автоматический рестарт приложения при смене ngrok URL

set -e

NGROK_API="http://localhost:4040/api/tunnels"
ENV_FILE=".env"
LAST_URL=""

echo "🔄 Запуск watch-ngrok.sh..."
echo "Мониторю ngrok URL на изменения..."

while true; do
  # Получаем текущий ngrok URL
  CURRENT_URL=$(curl -s "$NGROK_API" | grep -o '"public_url":"[^"]*' | cut -d'"' -f4 | head -1)
  
  if [ -z "$CURRENT_URL" ]; then
    echo "⚠️  ngrok не запущен. Жду 5 сек..."
    sleep 5
    continue
  fi
  
  # Если URL изменился
  if [ "$CURRENT_URL" != "$LAST_URL" ]; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "✅ Новый ngrok URL обнаружен: $CURRENT_URL"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    LAST_URL="$CURRENT_URL"
    
    # Обновляем .env
    echo "📝 Обновляю $ENV_FILE..."
    
    # Читаем текущий .env, обновляем BASE_URL и SPOTIFY_REDIRECT_URI
    if [ -f "$ENV_FILE" ]; then
      # Сохраняем все переменные, кроме BASE_URL и SPOTIFY_REDIRECT_URI
      grep -v "^BASE_URL=" "$ENV_FILE" | grep -v "^SPOTIFY_REDIRECT_URI=" > "$ENV_FILE.tmp"
      
      # Добавляем новые значения
      echo "BASE_URL=$CURRENT_URL" >> "$ENV_FILE.tmp"
      echo "SPOTIFY_REDIRECT_URI=$CURRENT_URL/auth/spotify/callback" >> "$ENV_FILE.tmp"
      
      mv "$ENV_FILE.tmp" "$ENV_FILE"
      
      echo "✅ $ENV_FILE обновлен"
      echo "   BASE_URL=$CURRENT_URL"
      echo "   SPOTIFY_REDIRECT_URI=$CURRENT_URL/auth/spotify/callback"
    fi
    
    # Перезапускаем приложение
    echo "🔄 Перезапускаю docker compose..."
    docker compose up --build -d
    
    echo "⏳ Жду 3 сек для инициализации приложения..."
    sleep 3
    
    # Проверяем здоровье
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
      echo "✅ Приложение готово на $CURRENT_URL"
    else
      echo "⚠️  Приложение еще не готово, жду..."
    fi
    
    echo ""
  fi
  
  sleep 3
done
