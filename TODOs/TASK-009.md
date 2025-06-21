# TASK-009: Cross-Region Sync

## Краткое описание
Реализовать батч-синхронизацию изменений мира между региональными узлами с delta-сжатием и приоритезацией очередей. Цель — гарантировать eventual consistency при минимальном межрегиональном трафике.

## Цели
1. `SyncBatch` структура: до 1000 изменений/5 МБ.
2. DeltaCompressor (ZSTD + protobuf packed)
3. Priority queue: блоки > сущности > чат.
4. Adjustable `sync_interval` (cli/env).
5. Метрики: `batch_size_bytes`, `region_sync_seconds`.
6. Резервирование: если узел недоступен, ретрай через экспон. backoff.

## Файлы/Папки
- `internal/sync/region_sync.go`
- `internal/network/proto/region_sync.proto`
- `internal/network/delta_compressor.go`
- Тесты: `tests/cross_region_sync_test.go`

## Подзадачи
1. [x] Базовый SyncBatch + BatchManager (накопление и отправка через EventBus)
2. [x] Приоритизация и дельта-компрессия блоков/энтити (gzip + passthrough)
3. [x] Написать `SyncProducer` (сбор изменений).
4. [x] Написать `SyncConsumer` (применение батча).
5. [x] Создать `SyncManager` для координации всех компонентов
6. [x] Интеграция в main.go с YAML конфигурацией
7. [x] Unit-тесты для компрессоров и BatchManager

## Приоритет
P1

## Сложность
4/5

## Зависимости
TASK-008

## Статус
New

## История изменений
- 2025-06-21: полная инфраструктура Cross-Region Sync (компрессия, producer/consumer, конфиг) 