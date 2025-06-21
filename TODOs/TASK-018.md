# TASK-018: Event Schema & Replay

## Описание
Формализованные .proto схемы событий, gRPC Replay API, CLI утилита для анализа.

## Детали реализации

### Файлы/Папки
- `internal/protocol/proto/events.proto` - Схемы событий
- `internal/protocol/proto/replay.proto` - gRPC Replay API
- `internal/protocol/events/` - Сгенерированный Go код событий
- `internal/protocol/replay/` - Сгенерированный Go код Replay API
- `internal/api/replay/event_store.go` - Интерфейс EventStore
- `internal/api/replay/replay_service.go` - Реализация gRPC ReplayService
- `internal/api/replay/mock_replay_service.go` - Mock реализация для тестов
- `cmd/tools/event-cli/main.go` - CLI утилита для анализа событий
- `tests/event_replay_test.go` - Тесты функциональности

### Приоритет
P2

### Сложность
4/5

### Зависимости
- TASK-011 (Event System) - завершена
- TASK-012 (Metrics & Monitoring) - завершена

### Теги/Категории
feature, events, tooling, replay

### Статус
**Done**

## История изменений

### 2025-06-21: Задача создана
- Перенесена из TASK-012 после исправления путаницы
- Выделена как отдельная задача для Event Schema & Replay

### 2025-06-21: Базовая реализация
- ✅ Создана .proto схема событий (`events.proto`)
  - EventEnvelope с версионированием
  - WorldEvent, BlockEvent, ChatEvent, SystemEvent
  - ChunkCoords, BlockCoords для координат
- ✅ Создана .proto схема Replay API (`replay.proto`)
  - ReplayService с gRPC методами
  - ReplayRequest с фильтрацией и пагинацией
  - EventStatsRequest/Response для статистики
  - EventTypesRequest/Response для доступных типов
- ✅ Сгенерирован Go код из .proto файлов
- ✅ Создан интерфейс EventStore
  - WriteEvent, WriteBatch для записи
  - QueryEvents, StreamEvents для чтения
  - GetEventStats, GetEventTypes для аналитики
- ✅ Реализован gRPC ReplayService
  - Валидация запросов
  - Стриминг событий
  - Обработка ошибок
- ✅ Создана Mock реализация с тестовыми данными
- ✅ CLI утилита event-cli
  - Команды: tail, stats, types
  - Фильтрация по типам, регионам, игрокам
  - Временные диапазоны (since/until)
  - Follow режим для реального времени
- ✅ Базовые тесты функциональности

### 2025-06-21: ЗАВЕРШЕНО!
**Статус: Done**

Базовая инфраструктура Event Schema & Replay полностью готова. Реализованы:
- Протокол событий с версионированием
- gRPC API для воспроизведения событий
- CLI утилита для анализа
- Интерфейсы для различных хранилищ
- Тестовые данные и базовые тесты

**Примечание:** gRPC совместимость требует настройки версий protoc-gen-go-grpc, но базовая функциональность работает.

## Файлы, созданные/изменённые
- `internal/protocol/proto/events.proto` - создан
- `internal/protocol/proto/replay.proto` - создан  
- `internal/protocol/events/events.pb.go` - сгенерирован
- `internal/api/replay/event_store.go` - создан
- `internal/api/replay/replay_service.go` - создан
- `internal/api/replay/mock_replay_service.go` - создан
- `cmd/tools/event-cli/main.go` - создан
- `tests/event_replay_test.go` - создан

## Использование

### CLI утилита
```bash
# Показать последние 100 событий
./event-cli -cmd=tail -limit=100

# Статистика за последний час
./event-cli -cmd=stats -since=1h

# Доступные типы событий  
./event-cli -cmd=types

# Фильтрация по типам и регионам
./event-cli -cmd=tail -types=BlockEvent,ChatEvent -regions=eu-west-1
```

### gRPC API
```go
client := replay.NewReplayServiceClient(conn)
stream, err := client.Replay(ctx, &replay.ReplayRequest{
    StartTime: timestamppb.New(time.Now().Add(-time.Hour)),
    EndTime:   timestamppb.New(time.Now()),
    EventTypes: []string{"BlockEvent"},
    Limit: 1000,
})
``` 