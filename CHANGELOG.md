# Changelog

## [v0.3.0-alpha] - 2025-06-21

### 🚀 Major Features

#### Многорегиональная архитектура
- **Regional Nodes** - Полная реализация региональных узлов для глобального мира
- **Cross-Region Sync** - Батч-синхронизация между регионами с delta compression
- **Conflict Resolution** - Автоматическое разрешение конфликтов (Last-Write-Wins)
- **Distributed Caching** - Redis hot cache + cold storage с NATS invalidation

#### Сетевая инфраструктура
- **KCP Protocol Migration** - Полный переход на KCP для игрового трафика
- **NetChannel System** - Гибкая система каналов (KCP, TCP, UDP, WebSocket)
- **Client-Side Prediction** - Предсказание действий на клиенте с серверной коррекцией
- **Adaptive Networking** - Адаптация под различные условия сети и пинг

#### Система событий и мониторинг
- **Event System** - NATS JetStream для событий и уведомлений
- **Event Replay** - Система воспроизведения событий с CLI утилитой
- **Prometheus Metrics** - Детальные метрики производительности (~50 метрик)
- **OpenTelemetry** - Распределенная трассировка HTTP/gRPC запросов
- **Structured Logging** - Профессиональное логирование по компонентам

### 🎮 Game Features

#### Game Actions System
- **9 типов действий**: INTERACT, ATTACK, USE_ITEM, PICKUP, DROP, BUILD_PLACE, BUILD_BREAK, EMOTE, RESPAWN
- **Валидация расстояний** - Проверка радиуса действий (2-5 блоков)
- **Система атак** - Урон, проверка здоровья, защита от самоатаки
- **Строительство** - Размещение и разрушение блоков с проверками

#### World System Improvements
- **Block Interaction** - Взаимодействие с блоками через OnInteract интерфейс
- **Entity Behavior** - Обновление поведения сущностей с физикой
- **Pending Events** - Неблокирующая обработка отложенных событий
- **Delta Transmission** - Эффективная отправка изменений блоков

### 🗄️ Storage & Performance

#### Redis Integration
- **Position Repository** - Позиции игроков в реальном времени
- **GEO Indexing** - Пространственные запросы с Redis GEO
- **Batching Operations** - Батчевые операции для производительности
- **Hot Cache** - Кэширование часто используемых данных

#### MariaDB Integration
- **Position Repository** - Долговременное хранение позиций
- **Transaction Support** - ACID транзакции для критических операций
- **Authentication** - Безопасное хранение пользователей

#### Spatial Indexing
- **5000+ Players Support** - Пространственное индексирование для масштабирования
- **Region Manager** - Эффективное управление регионами мира

### 🔧 Infrastructure

#### Logging Infrastructure
- **LoggerManager** - Централизованное управление логгерами
- **Component Loggers** - Специализированные логгеры (Network, Game, Regional, Sync)
- **Thread-Safe Operations** - Безопасная работа в многопоточной среде
- **Graceful Shutdown** - Корректное завершение всех логгеров

#### Channel Integration
- **TCP Channel** - Полная реализация с protobuf сериализацией
- **UDP/WebSocket Fallbacks** - Заглушки с возможностью расширения
- **Connection Statistics** - Метрики пакетов, байт, RTT
- **Event Handlers** - Обработчики подключения, отключения, ошибок

#### REST API & Authentication
- **JWT Authentication** - Безопасная аутентификация с HMAC-SHA256
- **Rate Limiting** - Middleware для ограничения запросов
- **Webhook System** - Outbound webhooks для интеграций
- **Replay API** - gRPC API для воспроизведения событий

### 🔨 DevOps & Tools

#### CI/CD Pipeline
- **GitHub Actions** - Автоматизация тестов, сборки и релизов
- **golangci-lint** - Статический анализ кода
- **Race Detection** - Автоматическое обнаружение гонок
- **Coverage Reports** - Отчеты о покрытии тестов

#### Configuration Management
- **Configurable Ports** - Настройка TCP/UDP/REST/Metrics портов
- **Multi-Region Configs** - Конфигурации для EU-West, US-East
- **Environment Variables** - Fallback на переменные окружения

#### CLI Tools
- **event-cli** - Утилита для анализа событий (tail, stats, types)
- **Multi-Region Testing** - Скрипты для тестирования мультирегиональности

### 📊 Performance Benchmarks
- **Regional Sync**: ~355K операций/сек
- **Event Processing**: высокая пропускная способность JetStream
- **NetChannel**: низкая задержка с KCP протоколом
- **Spatial Index**: эффективная обработка 5000+ игроков

### 🐛 Bug Fixes
- Исправлены все критические ошибки компиляции и линтера (40+ ошибок)
- Устранены гонки в менеджере сущностей
- Исправлена валидация сообщений и размеров данных
- Решены проблемы с блокировками в BigChunk

### 🔐 Security Improvements
- Валидация всех входящих данных
- Ограничения на размер метаданных (1КБ)
- Проверка радиуса действий игроков
- Защита от некорректных операций

### 📚 Documentation
- Обновлен README.md с полным описанием возможностей
- Создан docs/multi-region-setup.md
- Документация REST API и Webhook системы
- Подробные комментарии в коде

## [v0.0.2a] - 2025-06-07

### Security Fixes
- Перевод аутентификации на криптографически безопасные JWT (HMAC-SHA256, срок действия, валидация)
- Удалены тестовые и дефолтные учётные записи (test/test, admin/admin)
- Введено предупреждение о необходимости смены пароля для временного администратора
- Ограничения на количество подключений: максимум 1000 одновременных, не более 5 с одного IP, таймаут 5 минут
- Валидация входных данных: проверка позиции блока (радиус 10 блоков), допустимости ID блока, ограничение размера метаданных (1КБ)

### Performance & Stability
- Исправлена гонка в менеджере сущностей (блокировка на всё время обновления)
- Проверка размера сообщений до выделения памяти

### Architecture & Migration
- Миграция мира с 2D к многослойной 3D-структуре (Floor/Active/Ceiling)
- Введены BlockLayer, BlockCoord, методы Get/SetBlockLayer
- Расширен Chunk до Blocks3D[layer][x][y], обновлены метаданные и карты изменений
- Сохранена обратная совместимость

### DevOps & CI
- Добавлен .gitignore
- Настроен GitHub Actions: автоматизация тестов, сборки и релизов по тегу

### Прочее
- Все изменения проходят тесты и компиляцию
- Добавлены рекомендации по дальнейшему развитию: rate limiting, логирование, мониторинг, инструменты администратора, метрики 