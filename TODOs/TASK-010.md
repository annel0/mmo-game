# TASK-010: Distributed Caching

## Краткое описание
Ввести двухуровневое кеширование данных (Regional Hot Cache + Global Cold Storage) для ускорения доступа к часто изменяемым данным (блоки в активной зоне, инвентари).

## Цели
1. Redis/Memcached в регионе как Hot Cache (TTL 30с).
2. Global Store (MariaDB + S3 snapshots) как cold storage.
3. Cache invalidation через Pub/Sub (NATS).
4. Consistency model: Read-Through + Write-Behind (5с лаг).
5. Авто-прогрев кеша при старте узла.
6. Метрики: `cache_hit_ratio`, `write_behind_lag_sec`.

## Файлы/Папки
- `internal/cache/`
- `internal/storage/cache_repository.go`
- `internal/network/cache_invalidator.go`
- Тесты: `tests/cache_test.go`

## Подзадачи
1. [x] Спроектировать интерфейс `CacheRepo` (Get/Set/Invalidate).
2. [x] Реализовать Redis бэкенд.
3. [x] Реализовать Write-Behind в отдельной горутине.
4. [x] Pub/Sub invalidator (NATS).
5. [x] Авто-наполнение кеша при старте узла (Read-Through).
6. [x] Метрики и алерты.

## Приоритет
P2

## Сложность
3/5

## Зависимости
TASK-008, TASK-009

## Статус
Done

## История изменений
- 2025-06-21: файл создан.
- 2025-06-21: ЗАВЕРШЕНО! Полная Distributed Caching система

## Реализованные файлы
- `internal/cache/interface.go` — Интерфейсы и конфигурация
- `internal/cache/redis_cache.go` — Redis Hot Cache с Write-Behind
- `internal/cache/nats_invalidator.go` — NATS Pub/Sub инвалидация
- `tests/cache_test.go` — Comprehensive тесты и бенчмарки

## Особенности реализации
- **Двухуровневая архитектура**: Hot Cache (Redis) + Cold Storage (MariaDB)
- **Read-Through**: Автоматическая загрузка из Cold Storage при промахе
- **Write-Behind**: Асинхронная запись в Cold Storage с батчингом
- **Distributed Invalidation**: NATS Pub/Sub с дедупликацией
- **Comprehensive Metrics**: Hit ratio, latency, Write-Behind lag
- **Production Ready**: Graceful shutdown, error handling, reconnection 