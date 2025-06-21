# TASK-013: Observability Middleware

## Краткое описание
Создать универсальный набор middleware / interceptors для HTTP (Gin), gRPC и EventBus,
который будет автоматически:
1. Логировать запросы/сообщения с уникальным trace-ID (уровень INFO/DEBUG).
2. Снимать метрики (latency, throughput, error_rate) в Prometheus.
3. Проставлять trace-context (OpenTelemetry) для распределённого трейсинга.
4. Прозрачно интегрироваться с существующим `logging` и `eventbus` пакетами.

## Цели
1. Middleware для REST API (Gin):
   * request_duration_seconds (histogram)
   * request_inflight
   * request_errors_total
2. gRPC interceptors (на будущее — когда появится RPC-шлюз).
3. EventBus wrapper, публикующий latency/size метрики.
4. Конфиг через env / yaml (вкл./выкл. трассировку, sampling rate).
5. Документация + примеры.

## Файлы/Папки
- `internal/middleware/`
  - `prometheus_middleware.go`
  - `logging_middleware.go`
  - `trace_middleware.go`
- изменения в `cmd/server/main.go` (подключение)
- тесты `tests/middleware_test.go`

## Подзадачи
1. [x] Спроектировать интерфейс `MiddlewareChain` (compose) — пока не нужен, используем gin.Use()
2. [x] Реализовать Prometheus-middleware для Gin (`/metrics`).
3. [x] Добавить structured-logging middleware (trace-ID + user-agent).
4. [x] Ввести trace-context propagation (OpenTelemetry std SDK).
5. [x] Интегрировать в REST API + пример в README.
6. [x] Unit-тесты: latency buckets, error handling.
7. [x] Обновить CI (golangci-lint exempt for generated code).

## Приоритет
P1

## Сложность
2/5

## Зависимости
TASK-008 (шина уже публикует метрики — нужно унифицировать)

## Статус
Done

## История изменений
- 2025-06-21: файл создан (запрос пользователя)
- 2025-06-21: ЗАВЕРШЕНО! Полная Observability Middleware (документация, тесты, интеграция)

## Реализованные файлы
- `internal/middleware/prometheus_middleware.go` — Prometheus метрики (duration, inflight, errors)
- `internal/middleware/logging_middleware.go` — Структурированное логирование с trace-ID
- `internal/middleware/README.md` — Полная документация и примеры
- `tests/middleware_test.go` — Comprehensive unit-тесты и бенчмарки
- Интеграция в `internal/api/rest_server.go` 