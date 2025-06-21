# TASK-017: Player Experience Optimization

## Краткое описание
Оптимизировать клиентский опыт при высоком пинге: оптимистичное применение, плавные коррекции, адаптивная интерполяция.

## Цели
1. Optimistic updates на клиенте (apply immediately).
2. Gentle correction LERP 150–250 мс.
3. Adaptive snapshot rate (RTT-based).
4. Predictive path smoothing для движения.
5. UI latency indicators + настройка «Quality vs Smoothness».
6. Метрики: `prediction_error_px`, `corrections_per_min`.

## Файлы/Папки
- `client/prediction/` — новое пространство клиентских систем
- `internal/network/adaptive_snapshot.go`
- `internal/network/metrics_prediction.go`
- Тесты: `tests/player_experience_test.go`

## Подзадачи
1. [ ] Реализовать `AdaptiveSnapshotController`.
2. [ ] Улучшить `PredictionService` (сервер) для переменного тикрейта.
3. [ ] Клиент: LERP + тайм-индикатор задержки.
4. [ ] Benchmark: сравнить jitter до/после оптимизации.
5. [ ] Настраиваемый параметр в настройках клиента.

## Приоритет
P2

## Сложность
4/5

## Зависимости
TASK-001, TASK-002

## Теги/Категории
feature, UX, networking, client

## Статус
New

## История изменений
- 2025-06-21: задача создана (перенесена из TASK-011 после исправления путаницы) 