#!/bin/bash

# test-multi-region.sh
# Скрипт для тестирования мультирегиональной архитектуры MMO сервера

set -e

echo "🌍 Тестирование Multi-Region MMO архитектуры..."

# Проверяем наличие NATS
if ! curl -s http://localhost:8222/varz > /dev/null; then
    echo "❌ NATS Server недоступен на порту 8222!"
    echo "Запустите: docker run -d --name nats-server -p 4222:4222 -p 8222:8222 nats:latest -js"
    exit 1
fi

echo "✅ NATS Server запущен"

# Компилируем сервер если нет бинарника
if [ ! -f "./bin/mmo-server" ]; then
    echo "🔨 Компилирование сервера..."
    go build -o bin/mmo-server ./cmd/server
    echo "✅ Сервер скомпилирован"
fi

# Создаем логи
mkdir -p logs

echo "🚀 Запуск EU-West региона (порты: TCP 7777, UDP 7778, REST 8088, Metrics 2112)..."
GAME_CONFIG=config-eu-west.yml nohup ./bin/mmo-server > logs/eu-west.log 2>&1 &
EU_PID=$!
echo "EU-West PID: $EU_PID"

sleep 3

echo "🚀 Запуск US-East региона (порты: TCP 7779, UDP 7780, REST 8089, Metrics 2113)..."
# Теперь используем конфигурацию напрямую (без env vars)
GAME_CONFIG=config-us-east.yml nohup ./bin/mmo-server > logs/us-east.log 2>&1 &
US_PID=$!
echo "US-East PID: $US_PID"

sleep 5

echo "📊 Проверка статуса серверов..."

# Проверяем EU-West
if curl -s http://localhost:8088/health > /dev/null; then
    echo "✅ EU-West сервер работает (REST: 8088)"
else
    echo "❌ EU-West сервер недоступен"
fi

# Проверяем US-East  
if curl -s http://localhost:8089/health > /dev/null; then
    echo "✅ US-East сервер работает (REST: 8089)"
else
    echo "❌ US-East сервер недоступен"
fi

echo "📈 Проверка Prometheus метрик..."
EU_METRICS=$(curl -s http://localhost:2112/metrics | grep -c eventbus_ || echo "0")
US_METRICS=$(curl -s http://localhost:2113/metrics | grep -c eventbus_ || echo "0")
echo "  EU-West метрики: $EU_METRICS"
echo "  US-East метрики: $US_METRICS"

echo ""
echo "🎮 Серверы запущены! PIDs: EU-West=$EU_PID, US-East=$US_PID"
echo ""
echo "Для остановки выполните:"
echo "  kill $EU_PID $US_PID"
echo ""
echo "Логи доступны в:"
echo "  tail -f logs/eu-west.log"
echo "  tail -f logs/us-east.log"
echo ""
echo "REST API эндпоинты:"
echo "  EU-West: http://localhost:8088"
echo "  US-East: http://localhost:8089"
echo ""
echo "Prometheus метрики:"
echo "  EU-West: http://localhost:2112/metrics"  
echo "  US-East: http://localhost:2113/metrics"
echo ""
echo "Тестирование синхронизации:"
echo "  watch 'curl -s http://localhost:2112/metrics | grep eventbus_'" 