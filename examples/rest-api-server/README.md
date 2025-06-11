# REST API для MMO Game Server

Этот пример демонстрирует интеграцию REST API с игровым сервером, включая поддержку MariaDB для хранения пользователей и JWT аутентификацию.

## Функционал

### ✨ Основные возможности

- **JWT Аутентификация** - безопасная авторизация с токенами
- **MariaDB интеграция** - или in-memory репозиторий для разработки
- **Админ панель** - регистрация пользователей, управление
- **Статистика** - информация о сервере и игроках
- **Webhook поддержка** - для внешних интеграций
- **CORS** - для веб-клиентов
- **Graceful shutdown** - корректная остановка сервера

### 🔐 Эндпоинты

#### Публичные (без аутентификации)
- `GET /health` - проверка состояния сервера
- `POST /api/auth/login` - вход в систему

#### Защищенные (требуют JWT)
- `GET /api/stats` - статистика сервера
- `GET /api/server` - информация о сервере

#### Админские (требуют JWT + права админа)
- `POST /api/admin/register` - регистрация пользователя
- `GET /api/admin/users` - список пользователей
- `POST /api/admin/ban` - бан пользователя (заглушка)
- `POST /api/admin/unban` - разбан пользователя (заглушка)

#### Webhook
- `POST /api/webhook` - прием внешних событий

## Быстрый запуск

### 1. Запуск с in-memory БД (для разработки)

```bash
cd examples/rest-api-server
go run main.go
```

Сервер запустится на:
- 🎮 Игровой сервер: TCP :7777, UDP :7778
- 🌐 REST API: http://localhost:8080

### 2. Настройка MariaDB (для продакшена)

#### Установка MariaDB
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install mariadb-server

# CentOS/RHEL
sudo yum install mariadb-server

# macOS
brew install mariadb
```

#### Настройка базы данных
```sql
-- Подключитесь к MariaDB как root
mysql -u root -p

-- Создайте базу данных
CREATE DATABASE blockverse CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Создайте пользователя
CREATE USER 'gameuser'@'localhost' IDENTIFIED BY 'gamepass123';

-- Дайте права
GRANT ALL PRIVILEGES ON blockverse.* TO 'gameuser'@'localhost';
FLUSH PRIVILEGES;

-- Проверьте подключение
quit
mysql -u gameuser -p blockverse
```

#### Включение MariaDB в коде
```go
// В main.go измените:
UseMariaDB: true, // было false

// И настройте реальные данные:
MariaConfig: auth.MariaConfig{
    Host:     "localhost",
    Port:     3306,
    Database: "blockverse",
    Username: "gameuser",
    Password: "gamepass123",
},
```

## Примеры использования

### 1. Проверка состояния
```bash
curl http://localhost:8080/health
```

### 2. Вход в систему
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "admin",
    "password": "ChangeMe123!"
  }'
```

Ответ:
```json
{
  "success": true,
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "message": "Успешная авторизация",
  "user_id": 1,
  "is_admin": true
}
```

### 3. Получение статистики (требует JWT)
```bash
curl -H 'Authorization: Bearer YOUR_JWT_TOKEN' \
  http://localhost:8080/api/stats
```

### 4. Регистрация пользователя (только админы)
```bash
curl -X POST http://localhost:8080/api/admin/register \
  -H 'Authorization: Bearer YOUR_JWT_TOKEN' \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "newuser",
    "password": "securepass123",
    "is_admin": false
  }'
```

### 5. Получение информации о сервере
```bash
curl -H 'Authorization: Bearer YOUR_JWT_TOKEN' \
  http://localhost:8080/api/server
```

### 6. Webhook
```bash
curl -X POST http://localhost:8080/api/webhook \
  -H 'Content-Type: application/json' \
  -d '{
    "event": "player_joined",
    "player_id": 123,
    "timestamp": 1672531200
  }'
```

## Структура проекта

```
internal/
├── api/                    # REST API
│   ├── rest_server.go     # Основной сервер
│   ├── middleware.go      # JWT и админ middleware
│   └── integration.go     # Интеграция с игровым сервером
├── auth/                  # Аутентификация
│   ├── maria_repository.go # MariaDB репозиторий
│   ├── mongo_repository.go # MongoDB репозиторий (legacy)
│   ├── user_repo_memory.go # In-memory репозиторий
│   └── jwt.go            # JWT функции
└── ...
```

## Безопасность

### 🔒 Встроенные меры
- JWT токены с истечением срока действия
- Проверка прав доступа для админских функций
- Валидация входных данных
- Rate limiting (планируется)
- HTTPS поддержка (рекомендуется для продакшена)

### ⚠️ Важные замечания
- Измените пароль админа по умолчанию `ChangeMe123!`
- Используйте HTTPS в продакшене
- Настройте firewall для доступа к БД
- Регулярно обновляйте JWT секретный ключ

## Интеграция с веб-сайтом

Этот REST API готов для интеграции с веб-сайтом:

```javascript
// Пример фронтенд кода
const API_BASE = 'http://localhost:8080/api';

// Вход
const login = async (username, password) => {
  const response = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password })
  });
  return response.json();
};

// Получение статистики
const getStats = async (token) => {
  const response = await fetch(`${API_BASE}/stats`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  return response.json();
};
```

## Следующие шаги

1. **Rate Limiting** - ограничение количества запросов
2. **Логирование** - детальные логи API запросов
3. **Метрики** - Prometheus/Grafana интеграция
4. **Swagger документация** - автогенерация API docs
5. **Тесты** - unit и интеграционные тесты
6. **Docker** - контейнеризация для деплоя 