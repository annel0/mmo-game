package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/go-redis/redis/v8"
)

// RedisCache реализует CacheRepo используя Redis как Hot Cache.
// Поддерживает Write-Behind паттерн для асинхронной записи в Cold Storage.
//
// Особенности:
// - Автоматические метрики (hit ratio, latency)
// - Write-Behind с настраиваемым интервалом
// - Batch операции для производительности
// - Graceful shutdown
type RedisCache struct {
	client      *redis.Client
	config      *CacheConfig
	coldStorage ColdStorage
	invalidator CacheInvalidator

	// Write-Behind
	writeBehindQueue chan *writeItem
	writeBehindStop  chan struct{}
	writeBehindWg    sync.WaitGroup

	// Метрики
	metrics      *CacheMetrics
	metricsMutex sync.RWMutex

	// Статистика latency
	latencySum   int64 // в наносекундах
	latencyCount int64
	maxLatency   int64
}

// writeItem представляет элемент в очереди Write-Behind.
type writeItem struct {
	Key       string
	Value     []byte
	Timestamp time.Time
}

// NewRedisCache создаёт новый Redis кеш с опциональным Cold Storage.
//
// Параметры:
//
//	config - конфигурация Redis и Write-Behind
//	coldStorage - опциональное постоянное хранилище (может быть nil)
//	invalidator - опциональный invalidator для Pub/Sub (может быть nil)
//
// Возвращает:
//
//	*RedisCache - готовый к использованию кеш
//	error - ошибка подключения или конфигурации
func NewRedisCache(config *CacheConfig, coldStorage ColdStorage, invalidator CacheInvalidator) (*RedisCache, error) {
	// Настройки по умолчанию
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 30 * time.Second
	}
	if config.MaxTTL == 0 {
		config.MaxTTL = 1 * time.Hour
	}
	if config.WriteBehindInterval == 0 {
		config.WriteBehindInterval = 5 * time.Second
	}
	if config.WriteBehindBatchSize == 0 {
		config.WriteBehindBatchSize = 100
	}
	if config.MaxConnections == 0 {
		config.MaxConnections = 10
	}
	if config.PoolTimeout == 0 {
		config.PoolTimeout = 30 * time.Second
	}

	// Создаём Redis клиент
	rdb := redis.NewClient(&redis.Options{
		Addr:         config.RedisURL,
		Password:     config.RedisPassword,
		DB:           config.RedisDB,
		PoolSize:     config.MaxConnections,
		PoolTimeout:  config.PoolTimeout,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})

	// Проверяем соединение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	cache := &RedisCache{
		client:      rdb,
		config:      config,
		coldStorage: coldStorage,
		invalidator: invalidator,
		metrics: &CacheMetrics{
			LastUpdate: time.Now(),
		},
	}

	// Запускаем Write-Behind если включён
	if config.WriteBehindEnabled && coldStorage != nil {
		cache.writeBehindQueue = make(chan *writeItem, config.WriteBehindBatchSize*2)
		cache.writeBehindStop = make(chan struct{})
		cache.startWriteBehind()
	}

	logging.Info("Redis cache initialized: %s (Write-Behind: %v)", config.RedisURL, config.WriteBehindEnabled)
	return cache, nil
}

// Get получает значение по ключу из Redis кеша.
// При промахе пытается загрузить из Cold Storage (Read-Through).
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	start := time.Now()
	defer r.recordLatency(start)

	atomic.AddInt64(&r.metrics.TotalRequests, 1)

	// Попытка получить из Redis
	val, err := r.client.Get(ctx, key).Bytes()
	if err == nil {
		atomic.AddInt64(&r.metrics.CacheHits, 1)
		r.updateHitRatio()
		return val, nil
	}

	// Промах в Redis
	atomic.AddInt64(&r.metrics.CacheMisses, 1)

	if err != redis.Nil {
		logging.Error("Redis Get error for key %s: %v", key, err)
		r.updateHitRatio()
		return nil, fmt.Errorf("redis get error: %w", err)
	}

	// Read-Through: пытаемся загрузить из Cold Storage
	if r.coldStorage != nil {
		val, err := r.coldStorage.Load(ctx, key)
		if err == nil {
			// Загружаем в кеш для следующих запросов
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = r.Set(ctx, key, val, r.config.DefaultTTL)
			}()
			r.updateHitRatio()
			return val, nil
		}
		logging.Debug("Cold storage miss for key %s: %v", key, err)
	}

	r.updateHitRatio()
	return nil, ErrCacheMiss
}

// Set сохраняет значение в Redis кеше.
// Если включён Write-Behind, также ставит в очередь для записи в Cold Storage.
func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	start := time.Now()
	defer r.recordLatency(start)

	// Валидация TTL
	if ttl > r.config.MaxTTL {
		ttl = r.config.MaxTTL
	}

	// Записываем в Redis
	err := r.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		logging.Error("Redis Set error for key %s: %v", key, err)
		return fmt.Errorf("redis set error: %w", err)
	}

	// Write-Behind: ставим в очередь для записи в Cold Storage
	if r.config.WriteBehindEnabled && r.coldStorage != nil {
		select {
		case r.writeBehindQueue <- &writeItem{
			Key:       key,
			Value:     value,
			Timestamp: time.Now(),
		}:
		default:
			// Очередь полна, пишем синхронно
			logging.Warn("Write-behind queue full, writing synchronously: %s", key)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := r.coldStorage.Store(ctx, key, value); err != nil {
					logging.Error("Failed to write to cold storage: %v", err)
				}
			}()
		}
	}

	return nil
}

// Delete удаляет ключ из кеша и отправляет уведомление об инвалидации.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	start := time.Now()
	defer r.recordLatency(start)

	err := r.client.Del(ctx, key).Err()
	if err != nil {
		logging.Error("Redis Delete error for key %s: %v", key, err)
		return fmt.Errorf("redis delete error: %w", err)
	}

	// Отправляем уведомление об инвалидации
	if r.invalidator != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := r.invalidator.PublishInvalidation(ctx, key); err != nil {
				logging.Error("Failed to publish invalidation for key %s: %v", key, err)
			}
		}()
	}

	return nil
}

// Exists проверяет существование ключа в кеше.
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()
	defer r.recordLatency(start)

	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists error: %w", err)
	}

	return count > 0, nil
}

// Invalidate помечает ключ как недействительный и уведомляет другие узлы.
func (r *RedisCache) Invalidate(ctx context.Context, key string) error {
	return r.Delete(ctx, key)
}

// BatchGet получает несколько значений за один запрос.
func (r *RedisCache) BatchGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	start := time.Now()
	defer r.recordLatency(start)

	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	atomic.AddInt64(&r.metrics.TotalRequests, int64(len(keys)))

	pipe := r.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd)

	for _, key := range keys {
		cmds[key] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		logging.Error("Redis BatchGet pipeline error: %v", err)
		return nil, fmt.Errorf("redis batch get error: %w", err)
	}

	result := make(map[string][]byte)
	var hits, misses int64

	for key, cmd := range cmds {
		val, err := cmd.Bytes()
		if err == nil {
			result[key] = val
			hits++
		} else if err == redis.Nil {
			misses++
		} else {
			logging.Error("Redis BatchGet error for key %s: %v", key, err)
			misses++
		}
	}

	atomic.AddInt64(&r.metrics.CacheHits, hits)
	atomic.AddInt64(&r.metrics.CacheMisses, misses)
	r.updateHitRatio()

	return result, nil
}

// BatchSet сохраняет несколько значений за один запрос.
func (r *RedisCache) BatchSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	start := time.Now()
	defer r.recordLatency(start)

	if len(items) == 0 {
		return nil
	}

	// Валидация TTL
	if ttl > r.config.MaxTTL {
		ttl = r.config.MaxTTL
	}

	pipe := r.client.Pipeline()

	for key, value := range items {
		pipe.Set(ctx, key, value, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		logging.Error("Redis BatchSet pipeline error: %v", err)
		return fmt.Errorf("redis batch set error: %w", err)
	}

	// Write-Behind для всех элементов
	if r.config.WriteBehindEnabled && r.coldStorage != nil {
		for key, value := range items {
			select {
			case r.writeBehindQueue <- &writeItem{
				Key:       key,
				Value:     value,
				Timestamp: time.Now(),
			}:
			default:
				// Очередь полна, пропускаем
				logging.Warn("Write-behind queue full, skipping key: %s", key)
			}
		}
	}

	return nil
}

// Close закрывает соединение с Redis и останавливает Write-Behind.
func (r *RedisCache) Close() error {
	// Останавливаем Write-Behind
	if r.writeBehindStop != nil {
		close(r.writeBehindStop)
		r.writeBehindWg.Wait()
	}

	// Закрываем Redis соединение
	err := r.client.Close()
	if err != nil {
		logging.Error("Error closing Redis connection: %v", err)
		return err
	}

	logging.Info("Redis cache closed")
	return nil
}

// GetMetrics возвращает текущие метрики кеша.
func (r *RedisCache) GetMetrics() *CacheMetrics {
	r.metricsMutex.RLock()
	defer r.metricsMutex.RUnlock()

	// Копируем метрики для безопасности
	metrics := *r.metrics

	// Обновляем вычисляемые поля
	metrics.LastUpdate = time.Now()

	// Подсчитываем Write-Behind lag
	if r.writeBehindQueue != nil {
		metrics.PendingWrites = int64(len(r.writeBehindQueue))
		if metrics.PendingWrites > 0 {
			metrics.WriteBehindLagMs = int64(time.Since(time.Now()).Milliseconds())
		}
	}

	return &metrics
}

// startWriteBehind запускает горутину для асинхронной записи в Cold Storage.
func (r *RedisCache) startWriteBehind() {
	r.writeBehindWg.Add(1)
	go func() {
		defer r.writeBehindWg.Done()

		ticker := time.NewTicker(r.config.WriteBehindInterval)
		defer ticker.Stop()

		batch := make(map[string][]byte)

		for {
			select {
			case item := <-r.writeBehindQueue:
				batch[item.Key] = item.Value

				// Если batch заполнен, записываем
				if len(batch) >= r.config.WriteBehindBatchSize {
					r.flushWriteBehindBatch(batch)
					batch = make(map[string][]byte)
				}

			case <-ticker.C:
				// Периодически записываем накопленные данные
				if len(batch) > 0 {
					r.flushWriteBehindBatch(batch)
					batch = make(map[string][]byte)
				}

			case <-r.writeBehindStop:
				// Записываем оставшиеся данные перед выходом
				if len(batch) > 0 {
					r.flushWriteBehindBatch(batch)
				}
				return
			}
		}
	}()

	logging.Info("Write-Behind started (interval: %v, batch size: %d)",
		r.config.WriteBehindInterval, r.config.WriteBehindBatchSize)
}

// flushWriteBehindBatch записывает batch в Cold Storage.
func (r *RedisCache) flushWriteBehindBatch(batch map[string][]byte) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := r.coldStorage.BatchStore(ctx, batch)
	if err != nil {
		logging.Error("Write-Behind batch store failed (%d items): %v", len(batch), err)
	} else {
		logging.Debug("Write-Behind batch stored: %d items in %v", len(batch), time.Since(start))
	}
}

// recordLatency записывает latency метрику.
func (r *RedisCache) recordLatency(start time.Time) {
	latency := time.Since(start).Nanoseconds()

	atomic.AddInt64(&r.latencySum, latency)
	atomic.AddInt64(&r.latencyCount, 1)

	// Обновляем максимальную latency
	for {
		current := atomic.LoadInt64(&r.maxLatency)
		if latency <= current || atomic.CompareAndSwapInt64(&r.maxLatency, current, latency) {
			break
		}
	}

	// Периодически обновляем среднюю latency в метриках
	if atomic.LoadInt64(&r.latencyCount)%100 == 0 {
		r.updateLatencyMetrics()
	}
}

// updateLatencyMetrics обновляет метрики latency.
func (r *RedisCache) updateLatencyMetrics() {
	count := atomic.LoadInt64(&r.latencyCount)
	if count == 0 {
		return
	}

	sum := atomic.LoadInt64(&r.latencySum)
	max := atomic.LoadInt64(&r.maxLatency)

	r.metricsMutex.Lock()
	r.metrics.AvgLatencyMs = float64(sum) / float64(count) / 1e6 // нс в мс
	r.metrics.MaxLatencyMs = float64(max) / 1e6
	r.metricsMutex.Unlock()
}

// updateHitRatio обновляет hit ratio в метриках.
func (r *RedisCache) updateHitRatio() {
	hits := atomic.LoadInt64(&r.metrics.CacheHits)
	misses := atomic.LoadInt64(&r.metrics.CacheMisses)
	total := hits + misses

	if total > 0 {
		r.metricsMutex.Lock()
		r.metrics.HitRatio = float64(hits) / float64(total)
		r.metricsMutex.Unlock()
	}
}
