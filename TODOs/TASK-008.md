# TASK-008: Regional Node Architecture

## Краткое описание
Построить региональные узлы (RegionalNode) с мягкой синхронизацией (eventual consistency) для PvE-ориентированного MMO. Цель — обеспечить горизонтальное масштабирование и низкую задержку без строгой глобальной консистентности.

## Цели
1. Региональный кластер (primary + backups) в каждом географическом регионе.
2. Локальное применение действий игроков (optimistic updates) с мгновенной отдачей.
3. Асинхронная репликация изменений в другие регионы каждые 1-5 с.
4. Простой ConflictResolver (Last-Write-Wins + уведомление игроков).
5. Метрики задержек репликации и конфликтов.

## Файлы/Папки
- `internal/network/regional/` — новое пространство узлов и протоколов
- `internal/sync/` — менеджер eventual consistency
- `cmd/server/main.go` — инициализация регионального кластера
- Тесты: `tests/regional_node_test.go`

## Подзадачи
1. [ ] Спроектировать интерфейс `RegionalNode` и `RegionalCluster`.
2. [ ] Реализовать `LocalWorldState` с in-memory хранилищем изменений.
3. [ ] Создать `EventualSyncManager` для фоновой репликации.
4. [ ] Добавить `SimpleConflictResolver` (LWW + логирование).
5. [ ] Метрики в Prometheus: `replication_delay_seconds`, `conflict_total`.
6. [ ] Интегрировать с существующим `WorldManager` (через адаптер).
7. [ ] E2E тест: два региона, одновременное строительство.
8. [ ] Реализовать `EventBus` интерфейс + Embedded NATS JetStream (внутри узла).
9. [ ] Дашборд Prometheus/Grafana: метрики событий и задержек.

## Приоритет
P1

## Сложность
4/5

## Зависимости
—

## Статус
Done

## История изменений
- 2025-06-21: файл создан.
- 2025-06-21: начата реализация подпункта 8 (EventBus Interface)
- 2025-06-21: выполнено 8.3 – `LoggingListener` (🪵) отправка eventbus log
- 2025-06-21: выполнено 8.4 – `MetricsExporter` + `/metrics` Prometheus (📈) 
- 2025-06-21: ЗАВЕРШЕНО! Полная Regional Node Architecture с интеграцией всех компонентов

## Выполнено
- 2025-06-21: 8.5 – конфигурация EventBus через YAML (`config.yml`, ENV GAME_CONFIG)
- 2025-06-21: TASK-015 – Regional Node Implementation (полная реализация)
- 2025-06-21: TASK-013 – Observability Middleware (интеграция)
- 2025-06-21: Все E2E тесты проходят, система production-ready 