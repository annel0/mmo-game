package cache

import (
	"context"
	"time"
)

// CacheRepo определяет интерфейс для кеширования данных.
// Поддерживает двухуровневую архитектуру: Hot Cache (Redis) + Cold Storage (MariaDB).
//
// Использование:
//
//	cache := NewRedisCache(config)
//	data, err := cache.Get(ctx, "key")
//	err = cache.Set(ctx, "key", data, 30*time.Second)
//	err = cache.Invalidate(ctx, "key")
type CacheRepo interface {
	// Get получает значение по ключу из кеша.
	// Возвращает ErrCacheMiss если ключ не найден.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set сохраняет значение в кеше с указанным TTL.
	// TTL = 0 означает отсутствие истечения.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete удаляет ключ из кеша.
	Delete(ctx context.Context, key string) error

	// Exists проверяет существование ключа в кеше.
	Exists(ctx context.Context, key string) (bool, error)

	// Invalidate помечает ключ как недействительный и рассылает уведомление.
	Invalidate(ctx context.Context, key string) error

	// BatchGet получает несколько значений за один запрос.
	BatchGet(ctx context.Context, keys []string) (map[string][]byte, error)

	// BatchSet сохраняет несколько значений за один запрос.
	BatchSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error

	// Close закрывает соединение с кешем.
	Close() error

	// GetMetrics возвращает метрики кеша.
	GetMetrics() *CacheMetrics
}

// ColdStorage определяет интерфейс для постоянного хранения данных.
// Используется как fallback когда данные отсутствуют в Hot Cache.
type ColdStorage interface {
	// Load загружает данные из постоянного хранилища.
	Load(ctx context.Context, key string) ([]byte, error)

	// Store сохраняет данные в постоянное хранилище.
	Store(ctx context.Context, key string, value []byte) error

	// BatchLoad загружает несколько записей.
	BatchLoad(ctx context.Context, keys []string) (map[string][]byte, error)

	// BatchStore сохраняет несколько записей.
	BatchStore(ctx context.Context, items map[string][]byte) error

	// Close закрывает соединение с хранилищем.
	Close() error
}

// CacheInvalidator управляет инвалидацией кеша через Pub/Sub.
type CacheInvalidator interface {
	// PublishInvalidation отправляет уведомление об инвалидации.
	PublishInvalidation(ctx context.Context, key string) error

	// SubscribeInvalidations подписывается на уведомления об инвалидации.
	SubscribeInvalidations(ctx context.Context, handler InvalidationHandler) error

	// Close закрывает соединение.
	Close() error
}

// InvalidationHandler обрабатывает уведомления об инвалидации кеша.
type InvalidationHandler func(key string) error

// CacheMetrics содержит метрики производительности кеша.
type CacheMetrics struct {
	// Общие метрики
	TotalRequests int64   `json:"total_requests"`
	CacheHits     int64   `json:"cache_hits"`
	CacheMisses   int64   `json:"cache_misses"`
	HitRatio      float64 `json:"hit_ratio"`

	// Метрики производительности
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`

	// Метрики хранилища
	TotalKeys     int64   `json:"total_keys"`
	TotalMemoryMB float64 `json:"total_memory_mb"`

	// Write-Behind метрики
	WriteBehindLagMs int64 `json:"write_behind_lag_ms"`
	PendingWrites    int64 `json:"pending_writes"`

	// Последнее обновление
	LastUpdate time.Time `json:"last_update"`
}

// CacheConfig содержит конфигурацию для кеша.
type CacheConfig struct {
	// Redis конфигурация
	RedisURL      string `yaml:"redis_url" env:"CACHE_REDIS_URL"`
	RedisPassword string `yaml:"redis_password" env:"CACHE_REDIS_PASSWORD"`
	RedisDB       int    `yaml:"redis_db" env:"CACHE_REDIS_DB"`

	// TTL настройки
	DefaultTTL time.Duration `yaml:"default_ttl" env:"CACHE_DEFAULT_TTL"`
	MaxTTL     time.Duration `yaml:"max_ttl" env:"CACHE_MAX_TTL"`

	// Write-Behind конфигурация
	WriteBehindEnabled   bool          `yaml:"write_behind_enabled" env:"CACHE_WRITE_BEHIND_ENABLED"`
	WriteBehindInterval  time.Duration `yaml:"write_behind_interval" env:"CACHE_WRITE_BEHIND_INTERVAL"`
	WriteBehindBatchSize int           `yaml:"write_behind_batch_size" env:"CACHE_WRITE_BEHIND_BATCH_SIZE"`

	// Производительность
	MaxConnections int           `yaml:"max_connections" env:"CACHE_MAX_CONNECTIONS"`
	PoolTimeout    time.Duration `yaml:"pool_timeout" env:"CACHE_POOL_TIMEOUT"`

	// Мониторинг
	MetricsEnabled bool `yaml:"metrics_enabled" env:"CACHE_METRICS_ENABLED"`
}

// Ошибки кеша
var (
	ErrCacheMiss     = NewCacheError("cache miss")
	ErrCacheTimeout  = NewCacheError("cache timeout")
	ErrCacheConflict = NewCacheError("cache conflict")
	ErrInvalidKey    = NewCacheError("invalid key")
)

// CacheError представляет ошибку кеша.
type CacheError struct {
	Message string
}

func (e *CacheError) Error() string {
	return e.Message
}

func NewCacheError(message string) *CacheError {
	return &CacheError{Message: message}
}

// IsCacheMiss проверяет, является ли ошибка промахом кеша.
func IsCacheMiss(err error) bool {
	return err == ErrCacheMiss
}
