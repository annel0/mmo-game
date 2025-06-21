package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/nats-io/nats.go"
)

// NATSInvalidator реализует CacheInvalidator используя NATS Pub/Sub.
// Обеспечивает распределённую инвалидацию кеша между региональными узлами.
//
// Особенности:
// - Автоматическое переподключение при сбоях
// - Дедупликация сообщений
// - Graceful shutdown
// - Метрики публикации/подписки
type NATSInvalidator struct {
	conn    *nats.Conn
	config  *InvalidatorConfig
	subject string
	nodeID  string

	// Подписки
	subscription *nats.Subscription
	handler      InvalidationHandler

	// Graceful shutdown
	stopCh chan struct{}
	wg     sync.WaitGroup

	// Дедупликация
	recentKeys map[string]time.Time
	keysMutex  sync.RWMutex

	// Метрики (используем atomic для thread safety)
	publishedCount int64
	receivedCount  int64
	errorsCount    int64
}

// InvalidatorConfig содержит конфигурацию для NATS invalidator.
type InvalidatorConfig struct {
	// NATS подключение
	NATSURL string `yaml:"nats_url" env:"CACHE_NATS_URL"`
	Subject string `yaml:"subject" env:"CACHE_NATS_SUBJECT"`

	// Retry настройки
	MaxReconnects int           `yaml:"max_reconnects" env:"CACHE_NATS_MAX_RECONNECTS"`
	ReconnectWait time.Duration `yaml:"reconnect_wait" env:"CACHE_NATS_RECONNECT_WAIT"`

	// Дедупликация
	DedupeWindow time.Duration `yaml:"dedupe_window" env:"CACHE_NATS_DEDUPE_WINDOW"`

	// Timeouts
	PublishTimeout time.Duration `yaml:"publish_timeout" env:"CACHE_NATS_PUBLISH_TIMEOUT"`
}

// InvalidationMessage представляет сообщение об инвалидации кеша.
type InvalidationMessage struct {
	Key       string    `json:"key"`
	Timestamp time.Time `json:"timestamp"`
	NodeID    string    `json:"node_id"`
	Reason    string    `json:"reason,omitempty"`
}

// NewNATSInvalidator создаёт новый NATS invalidator.
//
// Параметры:
//
//	config - конфигурация NATS соединения
//	nodeID - уникальный идентификатор узла
//
// Возвращает:
//
//	*NATSInvalidator - готовый к использованию invalidator
//	error - ошибка подключения
func NewNATSInvalidator(config *InvalidatorConfig, nodeID string) (*NATSInvalidator, error) {
	// Настройки по умолчанию
	if config.Subject == "" {
		config.Subject = "cache.invalidation"
	}
	if config.MaxReconnects == 0 {
		config.MaxReconnects = 10
	}
	if config.ReconnectWait == 0 {
		config.ReconnectWait = 2 * time.Second
	}
	if config.DedupeWindow == 0 {
		config.DedupeWindow = 5 * time.Second
	}
	if config.PublishTimeout == 0 {
		config.PublishTimeout = 5 * time.Second
	}

	// Настройки NATS соединения
	opts := []nats.Option{
		nats.MaxReconnects(config.MaxReconnects),
		nats.ReconnectWait(config.ReconnectWait),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logging.Warn("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logging.Info("NATS reconnected to %s", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logging.Info("NATS connection closed")
		}),
	}

	// Подключаемся к NATS
	conn, err := nats.Connect(config.NATSURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	invalidator := &NATSInvalidator{
		conn:       conn,
		config:     config,
		subject:    config.Subject,
		nodeID:     nodeID,
		stopCh:     make(chan struct{}),
		recentKeys: make(map[string]time.Time),
	}

	// Запускаем очистку дедупликации
	invalidator.startDedupeCleanup()

	logging.Info("NATS invalidator initialized: %s (subject: %s)", config.NATSURL, config.Subject)
	return invalidator, nil
}

// PublishInvalidation отправляет уведомление об инвалидации ключа.
func (n *NATSInvalidator) PublishInvalidation(ctx context.Context, key string) error {
	// Проверяем дедупликацию
	if n.isDuplicate(key) {
		logging.Debug("Skipping duplicate invalidation for key: %s", key)
		return nil
	}

	msg := &InvalidationMessage{
		Key:       key,
		Timestamp: time.Now(),
		NodeID:    n.getNodeID(),
		Reason:    "cache_invalidation",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		atomic.AddInt64(&n.errorsCount, 1)
		return fmt.Errorf("failed to marshal invalidation message: %w", err)
	}

	// Публикуем с timeout
	ctx, cancel := context.WithTimeout(ctx, n.config.PublishTimeout)
	defer cancel()

	err = n.conn.Publish(n.subject, data)
	if err != nil {
		atomic.AddInt64(&n.errorsCount, 1)
		logging.Error("Failed to publish invalidation for key %s: %v", key, err)
		return fmt.Errorf("failed to publish invalidation: %w", err)
	}

	// Записываем в дедупликацию
	n.recordKey(key)
	atomic.AddInt64(&n.publishedCount, 1)

	logging.Debug("Published invalidation for key: %s", key)
	return nil
}

// SubscribeInvalidations подписывается на уведомления об инвалидации.
func (n *NATSInvalidator) SubscribeInvalidations(ctx context.Context, handler InvalidationHandler) error {
	if n.subscription != nil {
		return fmt.Errorf("already subscribed to invalidations")
	}

	n.handler = handler

	// Создаём подписку
	sub, err := n.conn.Subscribe(n.subject, n.handleInvalidationMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to invalidations: %w", err)
	}

	n.subscription = sub

	// Запускаем мониторинг контекста для graceful shutdown
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		select {
		case <-ctx.Done():
			n.unsubscribe()
		case <-n.stopCh:
			n.unsubscribe()
		}
	}()

	logging.Info("Subscribed to cache invalidations on subject: %s", n.subject)
	return nil
}

// Close закрывает соединение с NATS.
func (n *NATSInvalidator) Close() error {
	close(n.stopCh)
	n.wg.Wait()

	if n.subscription != nil {
		n.subscription.Unsubscribe()
	}

	n.conn.Close()
	logging.Info("NATS invalidator closed")
	return nil
}

// GetMetrics возвращает метрики invalidator.
func (n *NATSInvalidator) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"published_count": atomic.LoadInt64(&n.publishedCount),
		"received_count":  atomic.LoadInt64(&n.receivedCount),
		"errors_count":    atomic.LoadInt64(&n.errorsCount),
		"connected":       n.conn.IsConnected(),
		"status":          n.conn.Status(),
	}
}

// handleInvalidationMessage обрабатывает входящие сообщения об инвалидации.
func (n *NATSInvalidator) handleInvalidationMessage(msg *nats.Msg) {
	atomic.AddInt64(&n.receivedCount, 1)

	var invalidationMsg InvalidationMessage
	if err := json.Unmarshal(msg.Data, &invalidationMsg); err != nil {
		atomic.AddInt64(&n.errorsCount, 1)
		logging.Error("Failed to unmarshal invalidation message: %v", err)
		return
	}

	// Проверяем что это не наше собственное сообщение
	if invalidationMsg.NodeID == n.getNodeID() {
		logging.Debug("Ignoring own invalidation message for key: %s", invalidationMsg.Key)
		return
	}

	// Проверяем дедупликацию
	if n.isDuplicate(invalidationMsg.Key) {
		logging.Debug("Ignoring duplicate invalidation for key: %s", invalidationMsg.Key)
		return
	}

	// Записываем в дедупликацию
	n.recordKey(invalidationMsg.Key)

	// Вызываем обработчик
	if n.handler != nil {
		if err := n.handler(invalidationMsg.Key); err != nil {
			atomic.AddInt64(&n.errorsCount, 1)
			logging.Error("Invalidation handler failed for key %s: %v", invalidationMsg.Key, err)
		} else {
			logging.Debug("Processed invalidation for key: %s", invalidationMsg.Key)
		}
	}
}

// unsubscribe отписывается от уведомлений.
func (n *NATSInvalidator) unsubscribe() {
	if n.subscription != nil {
		if err := n.subscription.Unsubscribe(); err != nil {
			logging.Error("Failed to unsubscribe from invalidations: %v", err)
		} else {
			logging.Info("Unsubscribed from cache invalidations")
		}
		n.subscription = nil
	}
}

// isDuplicate проверяет, является ли ключ дублированным.
func (n *NATSInvalidator) isDuplicate(key string) bool {
	n.keysMutex.RLock()
	defer n.keysMutex.RUnlock()

	lastSeen, exists := n.recentKeys[key]
	if !exists {
		return false
	}

	// Проверяем окно дедупликации
	return time.Since(lastSeen) < n.config.DedupeWindow
}

// recordKey записывает ключ в дедупликацию.
func (n *NATSInvalidator) recordKey(key string) {
	n.keysMutex.Lock()
	defer n.keysMutex.Unlock()

	n.recentKeys[key] = time.Now()
}

// startDedupeCleanup запускает периодическую очистку дедупликации.
func (n *NATSInvalidator) startDedupeCleanup() {
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()

		ticker := time.NewTicker(n.config.DedupeWindow)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				n.cleanupDedupe()
			case <-n.stopCh:
				return
			}
		}
	}()
}

// cleanupDedupe удаляет старые записи из дедупликации.
func (n *NATSInvalidator) cleanupDedupe() {
	n.keysMutex.Lock()
	defer n.keysMutex.Unlock()

	now := time.Now()
	for key, timestamp := range n.recentKeys {
		if now.Sub(timestamp) > n.config.DedupeWindow {
			delete(n.recentKeys, key)
		}
	}

	logging.Debug("Dedupe cleanup completed, %d keys remaining", len(n.recentKeys))
}

// getNodeID возвращает идентификатор узла.
func (n *NATSInvalidator) getNodeID() string {
	return n.nodeID
}
