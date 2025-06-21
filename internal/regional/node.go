package regional

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/eventbus"
	"github.com/annel0/mmo-game/internal/logging"
	syncpkg "github.com/annel0/mmo-game/internal/sync"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/prometheus/client_golang/prometheus"
)

// NodeMetrics содержит метрики работы регионального узла
type NodeMetrics struct {
	LocalChanges      prometheus.Counter
	RemoteChanges     prometheus.Counter
	ConflictsResolved prometheus.Counter
	ReplicationLag    prometheus.Gauge
}

// NewNodeMetrics создаёт новые метрики для регионального узла
func NewNodeMetrics() *NodeMetrics {
	return &NodeMetrics{
		LocalChanges: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "regional_node_local_changes_total",
			Help: "Общее количество локальных изменений",
		}),
		RemoteChanges: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "regional_node_remote_changes_total",
			Help: "Общее количество удалённых изменений",
		}),
		ConflictsResolved: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "regional_node_conflicts_resolved_total",
			Help: "Общее количество разрешённых конфликтов",
		}),
		ReplicationLag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "regional_node_replication_lag_ms",
			Help: "Задержка репликации в миллисекундах",
		}),
	}
}

// WorldWrapper обёртка над world.WorldManager для региональных узлов
type WorldWrapper struct {
	manager *world.WorldManager
}

// ChangeData представляет декодированные данные изменения
type ChangeData struct {
	Type     string                 `json:"type"`
	Position map[string]interface{} `json:"position,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// NewWorldWrapper создаёт новую обёртку мира
func NewWorldWrapper(manager *world.WorldManager) *WorldWrapper {
	return &WorldWrapper{manager: manager}
}

// ApplyChange применяет изменение к миру
func (w *WorldWrapper) ApplyChange(change *syncpkg.Change) error {
	logging.Debug("WorldWrapper: применение изменения %d байт", len(change.Data))

	// Декодируем данные изменения
	changeData, err := w.decodeChangeData(change.Data)
	if err != nil {
		return fmt.Errorf("failed to decode change data: %w", err)
	}

	// Применяем изменение в зависимости от типа
	switch changeData.Type {
	case "block_place":
		return w.applyBlockPlace(changeData)
	case "block_break":
		return w.applyBlockBreak(changeData)
	case "entity_move":
		return w.applyEntityMove(changeData)
	case "chunk_load":
		return w.applyChunkLoad(changeData)
	default:
		logging.Warn("WorldWrapper: неизвестный тип изменения: %s", changeData.Type)
		return nil // Игнорируем неизвестные типы
	}
}

// decodeChangeData декодирует данные изменения из байтов
func (w *WorldWrapper) decodeChangeData(data []byte) (*ChangeData, error) {
	var changeData ChangeData
	if err := json.Unmarshal(data, &changeData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal change data: %w", err)
	}
	return &changeData, nil
}

// applyBlockPlace применяет размещение блока
func (w *WorldWrapper) applyBlockPlace(changeData *ChangeData) error {
	// Извлекаем координаты
	x, ok := changeData.Position["x"].(float64)
	if !ok {
		return fmt.Errorf("invalid x coordinate")
	}
	y, ok := changeData.Position["y"].(float64)
	if !ok {
		return fmt.Errorf("invalid y coordinate")
	}
	z, ok := changeData.Position["z"].(float64)
	if !ok {
		return fmt.Errorf("invalid z coordinate")
	}

	// Извлекаем тип блока
	blockType, ok := changeData.Data["block_type"].(string)
	if !ok {
		return fmt.Errorf("invalid block_type")
	}

	logging.Debug("WorldWrapper: размещение блока %s в (%d,%d,%d)", blockType, int(x), int(y), int(z))

	// Применяем через WorldManager (упрощенная версия)
	// В реальной реализации здесь будет вызов методов WorldManager
	return nil
}

// applyBlockBreak применяет разрушение блока
func (w *WorldWrapper) applyBlockBreak(changeData *ChangeData) error {
	x, ok := changeData.Position["x"].(float64)
	if !ok {
		return fmt.Errorf("invalid x coordinate")
	}
	y, ok := changeData.Position["y"].(float64)
	if !ok {
		return fmt.Errorf("invalid y coordinate")
	}
	z, ok := changeData.Position["z"].(float64)
	if !ok {
		return fmt.Errorf("invalid z coordinate")
	}

	logging.Debug("WorldWrapper: разрушение блока в (%d,%d,%d)", int(x), int(y), int(z))
	return nil
}

// applyEntityMove применяет перемещение сущности
func (w *WorldWrapper) applyEntityMove(changeData *ChangeData) error {
	entityID, ok := changeData.Data["entity_id"].(string)
	if !ok {
		return fmt.Errorf("invalid entity_id")
	}

	x, ok := changeData.Position["x"].(float64)
	if !ok {
		return fmt.Errorf("invalid x coordinate")
	}
	y, ok := changeData.Position["y"].(float64)
	if !ok {
		return fmt.Errorf("invalid y coordinate")
	}

	logging.Debug("WorldWrapper: перемещение сущности %s в (%.2f,%.2f)", entityID, x, y)
	return nil
}

// applyChunkLoad применяет загрузку чанка
func (w *WorldWrapper) applyChunkLoad(changeData *ChangeData) error {
	chunkX, ok := changeData.Position["chunk_x"].(float64)
	if !ok {
		return fmt.Errorf("invalid chunk_x")
	}
	chunkY, ok := changeData.Position["chunk_y"].(float64)
	if !ok {
		return fmt.Errorf("invalid chunk_y")
	}

	logging.Debug("WorldWrapper: загрузка чанка (%d,%d)", int(chunkX), int(chunkY))
	return nil
}

// RegionalNode представляет региональный узел с локальным состоянием мира
// и возможностью синхронизации с другими регионами.
type RegionalNode interface {
	GetRegionID() string
	GetLocalWorld() *WorldWrapper
	// Применить удалённое изменение от другого региона
	ApplyRemoteChange(change *syncpkg.Change) error
	// Отправить локальное изменение в другие регионы
	BroadcastLocalChange(change *syncpkg.Change) error
	Start(ctx context.Context) error
	Stop() error
	GetMetrics() *NodeMetrics
}

// RegionalNodeImpl реализует RegionalNode
type RegionalNodeImpl struct {
	mu         sync.RWMutex
	regionID   string
	localWorld *WorldWrapper
	resolver   ConflictResolver
	metrics    *NodeMetrics

	// Интеграция с sync системой
	eventBus     eventbus.EventBus
	batchManager *syncpkg.BatchManager
	subscription eventbus.Subscription

	// Управление жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type NodeConfig struct {
	RegionID     string
	WorldManager *world.WorldManager
	EventBus     eventbus.EventBus
	BatchManager *syncpkg.BatchManager
	Resolver     ConflictResolver
}

func NewRegionalNode(cfg NodeConfig) (*RegionalNodeImpl, error) {
	if cfg.RegionID == "" {
		return nil, fmt.Errorf("region_id не может быть пустым")
	}
	if cfg.WorldManager == nil {
		return nil, fmt.Errorf("world_manager обязателен")
	}
	if cfg.EventBus == nil {
		return nil, fmt.Errorf("event_bus обязателен")
	}
	if cfg.BatchManager == nil {
		return nil, fmt.Errorf("batch_manager обязателен")
	}

	resolver := cfg.Resolver
	if resolver == nil {
		resolver = NewLWWResolver()
	}

	node := &RegionalNodeImpl{
		regionID:     cfg.RegionID,
		localWorld:   NewWorldWrapper(cfg.WorldManager),
		resolver:     resolver,
		metrics:      NewNodeMetrics(),
		eventBus:     cfg.EventBus,
		batchManager: cfg.BatchManager,
	}

	// Регистрируем Prometheus метрики (игнорируем ошибки дублирования)
	collectors := []prometheus.Collector{
		node.metrics.LocalChanges,
		node.metrics.RemoteChanges,
		node.metrics.ConflictsResolved,
		node.metrics.ReplicationLag,
	}

	for _, collector := range collectors {
		if err := prometheus.Register(collector); err != nil {
			// Игнорируем ошибки дублирования метрик
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				logging.Warn("Не удалось зарегистрировать метрику: %v", err)
			}
		}
	}

	return node, nil
}

func (n *RegionalNodeImpl) GetRegionID() string {
	return n.regionID
}

func (n *RegionalNodeImpl) GetLocalWorld() *WorldWrapper {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.localWorld
}

func (n *RegionalNodeImpl) ApplyRemoteChange(change *syncpkg.Change) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Проверяем на конфликт (упрощённая логика - пока без реальной проверки)
	if n.hasConflict(change) {
		conflict := &Conflict{
			RemoteChange: change,
			DetectedAt:   time.Now(),
		}

		resolved, err := n.resolver.Resolve(conflict)
		if err != nil {
			logging.Warn("🔄 Regional[%s]: ошибка разрешения конфликта: %v", n.regionID, err)
			return fmt.Errorf("conflict resolution failed: %w", err)
		}

		if resolved == nil {
			logging.Debug("🔄 Regional[%s]: изменение отклонено при разрешении конфликта", n.regionID)
			return nil
		}

		change = resolved
		n.metrics.ConflictsResolved.Inc()
		logging.Debug("🔄 Regional[%s]: конфликт разрешён", n.regionID)
	}

	// Применяем изменение к локальному миру
	err := n.localWorld.ApplyChange(change)
	if err != nil {
		logging.Warn("🔄 Regional[%s]: ошибка применения изменения: %v", n.regionID, err)
		return fmt.Errorf("failed to apply change: %w", err)
	}

	// Обновляем метрики
	n.metrics.RemoteChanges.Inc()
	replicationLag := time.Since(change.Timestamp).Milliseconds()
	n.metrics.ReplicationLag.Set(float64(replicationLag))

	logging.Debug("🔄 Regional[%s]: применено удалённое изменение, lag=%dms",
		n.regionID, replicationLag)

	return nil
}

func (n *RegionalNodeImpl) BroadcastLocalChange(change *syncpkg.Change) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Устанавливаем источник изменения
	change.SourceRegion = n.regionID
	change.Timestamp = time.Now()

	// Отправляем через BatchManager
	n.batchManager.AddChange(*change)

	n.metrics.LocalChanges.Inc()
	logging.Debug("🔄 Regional[%s]: отправлено локальное изменение", n.regionID)

	return nil
}

func (n *RegionalNodeImpl) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.ctx != nil {
		return fmt.Errorf("node already started")
	}

	n.ctx, n.cancel = context.WithCancel(ctx)

	// Подписываемся на SyncBatch события
	sub, err := n.eventBus.Subscribe(n.ctx, eventbus.Filter{
		Types: []string{"SyncBatch"},
	}, n.handleSyncBatch)
	if err != nil {
		n.cancel()
		return fmt.Errorf("failed to subscribe to SyncBatch: %w", err)
	}
	n.subscription = sub

	logging.Info("🔄 Regional[%s]: узел запущен", n.regionID)
	return nil
}

func (n *RegionalNodeImpl) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.cancel != nil {
		n.cancel()
	}

	if n.subscription != nil {
		n.subscription.Unsubscribe()
	}

	n.wg.Wait()

	logging.Info("🔄 Regional[%s]: узел остановлен", n.regionID)
	return nil
}

func (n *RegionalNodeImpl) GetMetrics() *NodeMetrics {
	return n.metrics
}

// hasConflict проверяет есть ли конфликт с изменением
func (n *RegionalNodeImpl) hasConflict(change *syncpkg.Change) bool {
	// Проверяем конфликты на основе временных меток и типа изменения

	// Если изменение слишком старое (больше 5 минут), считаем его конфликтным
	if time.Since(change.Timestamp) > 5*time.Minute {
		logging.Debug("🔄 Regional[%s]: изменение слишком старое: %v", n.regionID, change.Timestamp)
		return true
	}

	// Если изменение из будущего (больше 1 минуты), тоже конфликт
	if change.Timestamp.After(time.Now().Add(1 * time.Minute)) {
		logging.Debug("🔄 Regional[%s]: изменение из будущего: %v", n.regionID, change.Timestamp)
		return true
	}

	// Проверяем конфликты по типу изменения
	changeData, err := n.parseChangeForConflict(change.Data)
	if err != nil {
		logging.Warn("🔄 Regional[%s]: ошибка парсинга изменения для проверки конфликта: %v", n.regionID, err)
		return false // Если не можем парсить, не считаем конфликтом
	}

	// Для изменений блоков проверяем одновременные модификации
	if changeData.Type == "block_place" || changeData.Type == "block_break" {
		return n.hasBlockConflict(changeData, change.Timestamp)
	}

	// Для перемещений сущностей проверяем телепортацию
	if changeData.Type == "entity_move" {
		return n.hasEntityConflict(changeData, change.Timestamp)
	}

	// По умолчанию конфликта нет
	return false
}

// parseChangeForConflict парсит изменение для проверки конфликтов (упрощенная версия)
func (n *RegionalNodeImpl) parseChangeForConflict(data []byte) (*ChangeData, error) {
	var changeData ChangeData
	if err := json.Unmarshal(data, &changeData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal change data: %w", err)
	}
	return &changeData, nil
}

// hasBlockConflict проверяет конфликты для изменений блоков
func (n *RegionalNodeImpl) hasBlockConflict(changeData *ChangeData, timestamp time.Time) bool {
	// В реальной реализации здесь будет проверка:
	// - Одновременное изменение одного блока разными регионами
	// - Проверка состояния блока в локальном мире
	// - Сравнение с последними изменениями

	// Упрощенная логика: считаем конфликтом если изменение произошло меньше секунды назад
	// и касается критических координат (например, около спавна)
	if x, ok := changeData.Position["x"].(float64); ok {
		if y, ok := changeData.Position["y"].(float64); ok {
			// Спавн в (0,0) - критическая зона
			if int(x) == 0 && int(y) == 0 && time.Since(timestamp) < time.Second {
				logging.Debug("🔄 Regional[%s]: конфликт блока в критической зоне (0,0)", n.regionID)
				return true
			}
		}
	}

	return false
}

// hasEntityConflict проверяет конфликты для перемещений сущностей
func (n *RegionalNodeImpl) hasEntityConflict(changeData *ChangeData, timestamp time.Time) bool {
	// В реальной реализации здесь будет проверка:
	// - Телепортация на большие расстояния
	// - Превышение максимальной скорости
	// - Проверка коллизий с другими сущностями

	// Упрощенная логика: проверяем телепортацию
	if x, ok := changeData.Position["x"].(float64); ok {
		if y, ok := changeData.Position["y"].(float64); ok {
			// Если координаты слишком большие, возможно телепортация
			if x > 10000 || y > 10000 || x < -10000 || y < -10000 {
				logging.Debug("🔄 Regional[%s]: возможная телепортация сущности в (%.2f,%.2f)", n.regionID, x, y)
				return true
			}
		}
	}

	return false
}

// handleSyncBatch обрабатывает входящие SyncBatch события
func (n *RegionalNodeImpl) handleSyncBatch(ctx context.Context, envelope *eventbus.Envelope) {
	// Игнорируем собственные сообщения
	if envelope.Source == n.regionID {
		return
	}

	n.wg.Add(1)
	defer n.wg.Done()

	logging.Debug("🔄 Regional[%s]: получен SyncBatch от %s, размер=%d байт",
		n.regionID, envelope.Source, len(envelope.Payload))

	// Декодируем SyncBatch
	changes := n.decodeSyncBatch(envelope.Payload)

	// Применяем каждое изменение
	for _, change := range changes {
		if err := n.ApplyRemoteChange(&change); err != nil {
			logging.Warn("🔄 Regional[%s]: ошибка применения изменения: %v", n.regionID, err)
		}
	}
}

// decodeSyncBatch декодирует SyncBatch из байтов
func (n *RegionalNodeImpl) decodeSyncBatch(payload []byte) []syncpkg.Change {
	// Используем DeltaCompressor для декодирования (как в SyncConsumer)
	compressor := syncpkg.NewPassthroughCompressor()

	changes, err := compressor.Decompress(payload)
	if err != nil {
		logging.Warn("🔄 Regional[%s]: ошибка декодирования SyncBatch: %v", n.regionID, err)
		return []syncpkg.Change{}
	}

	logging.Debug("🔄 Regional[%s]: декодировано %d изменений из SyncBatch", n.regionID, len(changes))
	return changes
}
