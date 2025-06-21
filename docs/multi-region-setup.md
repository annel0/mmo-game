# Запуск Multi-Region MMO сервера

Данная инструкция описывает, как запустить два или более региональных узла MMO сервера для тестирования Cross-Region синхронизации.

## Предварительные требования

1. **NATS Server** — для EventBus коммуникации между регионами
2. **Go 1.21+** — для компиляции сервера
3. **Порты**: убедитесь, что свободны порты для каждого региона

## Шаг 1: Установка и запуск NATS

### Через Docker:
```bash
docker run -d --name nats-server -p 4222:4222 -p 8222:8222 nats:latest -js
```

### Через бинарный файл:
```bash
# Скачать NATS server
wget https://github.com/nats-io/nats-server/releases/latest/download/nats-server-v2.10.4-linux-amd64.zip
unzip nats-server-v2.10.4-linux-amd64.zip
./nats-server -js
```

Проверьте работу NATS:
```bash
curl http://localhost:8222/varz | jq .
```

## Шаг 2: Подготовка конфигураций

Создайте конфигурационные файлы для каждого региона:

### `config-eu-west.yml`:
```yaml
eventbus:
  url: "nats://127.0.0.1:4222"
  stream: "GLOBAL_EVENTS"
  retention_hours: 24

sync:
  region_id: "eu-west-1"
  batch_size: 100
  flush_every_seconds: 3
  use_gzip_compression: true
```

### `config-us-east.yml`:
```yaml
eventbus:
  url: "nats://127.0.0.1:4222"
  stream: "GLOBAL_EVENTS"
  retention_hours: 24

sync:
  region_id: "us-east-1"
  batch_size: 100
  flush_every_seconds: 3
  use_gzip_compression: true
```

## Шаг 3: Компиляция сервера

```bash
cd /path/to/golang
go build -o bin/mmo-server ./cmd/server
```

## Шаг 4: Запуск региональных узлов

### Терминал 1 (EU-West регион):
```bash
export GAME_CONFIG=config-eu-west.yml
./bin/mmo-server
```

Сервер запустится на:
- **Game Server**: TCP :7777, UDP :7778
- **REST API**: :8088
- **Prometheus**: :2112

### Терминал 2 (US-East регион):
```bash
export GAME_CONFIG=config-us-east.yml
./bin/mmo-server
```

⚠️ **Конфликт портов**: Второй сервер не сможет запуститься на тех же портах!

## Шаг 5: Конфигурация портов для второго региона

Для решения конфликта портов, модифицируйте `config-us-east.yml`:

```yaml
eventbus:
  url: "nats://127.0.0.1:4222"
  stream: "GLOBAL_EVENTS"
  retention_hours: 24

sync:
  region_id: "us-east-1"
  batch_size: 100
  flush_every_seconds: 3
  use_gzip_compression: true

# TODO: Добавить секции для кастомизации портов
# server:
#   tcp_port: 7779
#   udp_port: 7780
#   rest_port: 8089
#   metrics_port: 2113
```

**Временное решение** — запуск с переменными окружения:
```bash
# Терминал 2 (пока не реализованы порты в конфиге)
export GAME_CONFIG=config-us-east.yml
export GAME_TCP_PORT=7779
export GAME_UDP_PORT=7780  
export GAME_REST_PORT=8089
export GAME_METRICS_PORT=2113
./bin/mmo-server
```

## Шаг 6: Проверка синхронизации

### Мониторинг EventBus метрик:
```bash
# EU-West metrics
curl http://localhost:2112/metrics | grep eventbus_

# US-East metrics  
curl http://localhost:2113/metrics | grep eventbus_
```

### Проверка логов синхронизации:
В логах должны появляться сообщения:
```
[INFO] 🔄 SyncManager инициализирован: region=eu-west-1, batch=100, flush=3s
[DEBUG] SyncConsumer: batch size=156 bytes from us-east-1
[DEBUG] SyncConsumer: decoded 3 changes
```

### Тестирование через REST API:
```bash
# Логин в EU-West
curl -X POST http://localhost:8088/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"ChangeMe123!"}'

# Логин в US-East  
curl -X POST http://localhost:8089/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"ChangeMe123!"}'
```

## Шаг 7: Симуляция изменений мира

Для генерации событий синхронизации можно:

1. **Подключить игровых клиентов** к разным регионам
2. **Использовать REST API** для отправки событий
3. **Имитировать блоковые изменения** через внутренние API

### Пример отправки события через REST:
```bash
TOKEN="your-admin-jwt-token"
curl -X POST http://localhost:8088/api/admin/events/send \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "BlockEvent",
    "data": {"x": 100, "y": 50, "block_type": "stone"}
  }'
```

## Диагностика проблем

### NATS недоступен:
```
❌ Не удалось инициализировать JetStreamBus: nats connect: connection refused
```
**Решение**: Запустить NATS server на порту 4222

### Конфликт портов:
```
❌ Ошибка запуска REST API: listen tcp :8088: bind: address already in use
```
**Решение**: Использовать разные порты для каждого региона

### Отсутствие синхронизации:
```
[DEBUG] SyncConsumer: batch size=0 bytes from unknown
```
**Решение**: Проверить `region_id` в конфигурации и подключение к NATS

## Производительность

Для оптимизации синхронизации:

1. **Увеличьте `flush_every_seconds`** для снижения частоты батчей
2. **Включите `use_gzip_compression: true`** для экономии трафика  
3. **Настройте `batch_size`** в зависимости от объёма изменений
4. **Используйте SSD** для NATS JetStream persistence

## Масштабирование

Для добавления третьего региона (Asia-Pacific):

### `config-ap-south.yml`:
```yaml
eventbus:
  url: "nats://127.0.0.1:4222"
  stream: "GLOBAL_EVENTS"  
  retention_hours: 24

sync:
  region_id: "ap-south-1"
  batch_size: 150
  flush_every_seconds: 2
  use_gzip_compression: true
```

Все три региона будут автоматически синхронизироваться через общий NATS stream `GLOBAL_EVENTS`.

---

**Успешного тестирования Multi-Region архитектуры!** 🌍 