# 📤 Система исходящих Webhook'ов

## 🎯 Описание

Система исходящих webhook'ов позволяет игровому серверу автоматически отправлять уведомления о различных событиях на внешние HTTP серверы. Это полезно для:

- 🔍 **Мониторинга** - уведомления о нагрузке, ошибках, производительности
- 🛡️ **Античита** - алерты о нарушениях и подозрительной активности  
- 👥 **Игровых событий** - подключения/отключения игроков, чат
- 🚨 **Алертов** - критические ошибки, проблемы с безопасностью
- 📊 **Интеграций** - Discord боты, Telegram уведомления, внешние панели

## 🏗️ Архитектура

```
┌─────────────────┐    📤    ┌─────────────────┐    🌐    ┌─────────────────┐
│  Игровой сервер │ ───────► │ Webhook Manager │ ───────► │ Внешние сервисы │
│                 │          │                 │          │                 │
│ • Античит       │          │ • Очередь       │          │ • Discord       │
│ • Игроки        │          │ • Retry логика  │          │ • Мониторинг    │
│ • Сервер        │          │ • Подписки      │          │ • Аналитика     │
└─────────────────┘          └─────────────────┘          └─────────────────┘
```

## 🚀 REST API Эндпоинты

### Список webhook'ов
```bash
GET /api/admin/webhooks
Authorization: Bearer <JWT_TOKEN>
```

### Создание webhook'а
```bash
POST /api/admin/webhooks
Authorization: Bearer <JWT_TOKEN>
Content-Type: application/json

{
  "name": "Discord Notifications",
  "url": "https://your-server.com/webhook",
  "events": ["player.joined", "anticheat.violation"],
  "secret": "optional_secret_key",
  "timeout": 30,
  "retry_count": 3
}
```

### Получение webhook'а
```bash
GET /api/admin/webhooks/{id}
Authorization: Bearer <JWT_TOKEN>
```

### Обновление webhook'а
```bash
PUT /api/admin/webhooks/{id}
Authorization: Bearer <JWT_TOKEN>
Content-Type: application/json

{
  "name": "Updated Name",
  "active": false
}
```

### Удаление webhook'а
```bash
DELETE /api/admin/webhooks/{id}
Authorization: Bearer <JWT_TOKEN>
```

### Тест webhook'а
```bash
POST /api/admin/webhooks/{id}/test
Authorization: Bearer <JWT_TOKEN>
```

### Доступные типы событий
```bash
GET /api/admin/webhooks/events
Authorization: Bearer <JWT_TOKEN>
```

## 📋 Типы событий

### 🖥️ Серверные события
- `server.started` - Запуск сервера
- `server.stopped` - Остановка сервера  
- `server.error` - Критическая ошибка
- `server.high_cpu` - Высокая нагрузка CPU
- `server.high_memory` - Высокое потребление памяти
- `server.low_tps` - Низкий TPS (производительность)

### 👤 События игроков
- `player.joined` - Подключение игрока
- `player.left` - Отключение игрока
- `player.banned` - Бан игрока
- `player.kicked` - Кик игрока

### 🛡️ Античит события
- `anticheat.violation` - Нарушение правил
- `anticheat.ban` - Автоматический бан

### 🌍 События мира
- `world.saved` - Сохранение мира
- `world.load_error` - Ошибка загрузки

### 💬 Другие события
- `chat.message` - Сообщение в чате
- `admin.command` - Админ команда
- `security.alert` - Предупреждение безопасности
- `backup.completed` - Резервная копия создана
- `backup.failed` - Ошибка резервного копирования

## 📦 Формат события

```json
{
  "event_type": "player.joined",
  "timestamp": 1749468130,
  "server_id": "game_server_01",
  "source": "game_server",
  "environment": "development",
  "data": {
    "username": "player1",
    "ip": "192.168.1.100",
    "time": 1749468130
  }
}
```

## 🔒 Безопасность

### HMAC подписи
Webhook'и могут быть подписаны секретным ключом:

```bash
# Заголовок с подписью
X-Webhook-Signature: sha256=abc123def456...
```

### Проверка подписи (Python)
```python
import hmac
import hashlib

def verify_signature(payload, secret, signature):
    expected = 'sha256=' + hmac.new(
        secret.encode(), 
        payload.encode(), 
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)
```

## 🔄 Retry логика

- ✅ **Автоматические повторы** при ошибках
- ⏰ **Экспоненциальная задержка** между попытками
- 📊 **Счетчик неудач** для каждого webhook'а
- 🚫 **Деактивация** при критическом количестве ошибок

## 📈 Мониторинг

### Метрики webhook'ов
- `last_used` - Время последней отправки
- `failure_count` - Количество неудач
- `active` - Активен ли webhook

### Проверка состояния
```bash
curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:8088/api/admin/webhooks | jq '.data.webhooks[].failure_count'
```

## 🛠️ Примеры использования

### Discord бот
```javascript
// Discord Webhook
const webhook = {
  name: "Discord Bot",
  url: "https://discord.com/api/webhooks/...",
  events: ["anticheat.violation", "player.banned"],
  timeout: 10,
  retry_count: 2
}
```

### Мониторинг Grafana
```yaml
# Grafana Webhook
webhook:
  name: "Grafana Alerts"
  url: "https://grafana.company.com/webhook"
  events: ["server.high_cpu", "server.error"]
  secret: "monitoring_secret"
```

### Telegram уведомления
```python
# Telegram Bot Webhook
import requests

def handle_webhook(event):
    if event['event_type'] == 'anticheat.violation':
        send_telegram_alert(event['data'])
```

## 🔧 Конфигурация

### Переменные окружения
```bash
# Webhook настройки
WEBHOOK_TIMEOUT=30
WEBHOOK_MAX_RETRIES=3
WEBHOOK_SECRET_KEY=your_secret_key

# Сервер настройки  
SERVER_ID=game_server_01
ENVIRONMENT=production
```

### Лимиты
- **Максимум webhook'ов**: 50 на сервер
- **Максимум событий**: 20 типов на webhook
- **Таймаут**: 1-120 секунд
- **Повторы**: 0-10 попыток

## 🚨 Устранение проблем

### Webhook не получает события
1. ✅ Проверьте URL webhook'а
2. ✅ Убедитесь что webhook активен
3. ✅ Проверьте подписку на нужные события
4. ✅ Посмотрите логи сервера

### Высокий failure_count
1. 🔍 Проверьте доступность endpoint'а
2. ⏰ Увеличьте timeout
3. 🔄 Добавьте больше retry попыток
4. 🛠️ Исправьте обработку в вашем сервисе

### Дублирование событий
1. 📝 Проверьте что webhook не подписан дважды
2. 🔀 Убедитесь в idempotency вашего обработчика
3. 📊 Используйте `timestamp` для дедупликации

## 📞 Поддержка

При возникновении вопросов:
1. 📖 Изучите логи игрового сервера
2. 🔍 Проверьте статус webhook'ов через API
3. 🧪 Используйте тестовые события для отладки
4. 📨 Обратитесь к администратору сервера

---

**💡 Совет**: Начните с простого webhook'а для тестирования, а затем расширяйте функциональность по мере необходимости! 