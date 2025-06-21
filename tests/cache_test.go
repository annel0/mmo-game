package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/annel0/mmo-game/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockColdStorage реализует cache.ColdStorage для тестов.
type MockColdStorage struct {
	data  map[string][]byte
	mutex sync.RWMutex
}

func NewMockColdStorage() *MockColdStorage {
	return &MockColdStorage{
		data: make(map[string][]byte),
	}
}

func (m *MockColdStorage) Load(ctx context.Context, key string) ([]byte, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if val, exists := m.data[key]; exists {
		return val, nil
	}
	return nil, fmt.Errorf("key not found: %s", key)
}

func (m *MockColdStorage) Store(ctx context.Context, key string, value []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data[key] = value
	return nil
}

func (m *MockColdStorage) BatchLoad(ctx context.Context, keys []string) (map[string][]byte, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	result := make(map[string][]byte)
	for _, key := range keys {
		if val, exists := m.data[key]; exists {
			result[key] = val
		}
	}
	return result, nil
}

func (m *MockColdStorage) BatchStore(ctx context.Context, items map[string][]byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for key, value := range items {
		m.data[key] = value
	}
	return nil
}

func (m *MockColdStorage) Close() error {
	return nil
}

// MockInvalidator реализует cache.CacheInvalidator для тестов.
type MockInvalidator struct {
	published []string
	handler   cache.InvalidationHandler
	mutex     sync.RWMutex
}

func NewMockInvalidator() *MockInvalidator {
	return &MockInvalidator{
		published: make([]string, 0),
	}
}

func (m *MockInvalidator) PublishInvalidation(ctx context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.published = append(m.published, key)
	return nil
}

func (m *MockInvalidator) SubscribeInvalidations(ctx context.Context, handler cache.InvalidationHandler) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.handler = handler
	return nil
}

func (m *MockInvalidator) Close() error {
	return nil
}

// SimulateInvalidation симулирует получение уведомления об инвалидации.
func (m *MockInvalidator) SimulateInvalidation(key string) error {
	m.mutex.RLock()
	handler := m.handler
	m.mutex.RUnlock()

	if handler != nil {
		return handler(key)
	}
	return nil
}

// GetPublished возвращает копию списка опубликованных ключей для тестирования.
func (m *MockInvalidator) GetPublished() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Возвращаем копию для безопасности
	result := make([]string, len(m.published))
	copy(result, m.published)
	return result
}

func TestRedisCache_BasicOperations(t *testing.T) {
	// Пропускаем если Redis недоступен
	config := &cache.CacheConfig{
		RedisURL:   "localhost:6379",
		DefaultTTL: 10 * time.Second,
	}

	redisCache, err := cache.NewRedisCache(config, nil, nil)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Test Set/Get
	key := "test:key1"
	value := []byte("test value 1")

	err = redisCache.Set(ctx, key, value, 5*time.Second)
	require.NoError(t, err)

	retrieved, err := redisCache.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, retrieved)

	// Test Exists
	exists, err := redisCache.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Test Delete
	err = redisCache.Delete(ctx, key)
	require.NoError(t, err)

	exists, err = redisCache.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Test cache miss
	_, err = redisCache.Get(ctx, "nonexistent")
	assert.True(t, cache.IsCacheMiss(err))
}

func TestRedisCache_BatchOperations(t *testing.T) {
	config := &cache.CacheConfig{
		RedisURL:   "localhost:6379",
		DefaultTTL: 10 * time.Second,
	}

	redisCache, err := cache.NewRedisCache(config, nil, nil)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Test BatchSet
	items := map[string][]byte{
		"batch:key1": []byte("value1"),
		"batch:key2": []byte("value2"),
		"batch:key3": []byte("value3"),
	}

	err = redisCache.BatchSet(ctx, items, 5*time.Second)
	require.NoError(t, err)

	// Test BatchGet
	keys := []string{"batch:key1", "batch:key2", "batch:key3", "batch:nonexistent"}
	result, err := redisCache.BatchGet(ctx, keys)
	require.NoError(t, err)

	assert.Equal(t, 3, len(result))
	assert.Equal(t, []byte("value1"), result["batch:key1"])
	assert.Equal(t, []byte("value2"), result["batch:key2"])
	assert.Equal(t, []byte("value3"), result["batch:key3"])
	assert.NotContains(t, result, "batch:nonexistent")
}

func TestRedisCache_ReadThrough(t *testing.T) {
	coldStorage := NewMockColdStorage()
	coldStorage.Store(context.Background(), "cold:key1", []byte("cold value"))

	config := &cache.CacheConfig{
		RedisURL:   "localhost:6379",
		DefaultTTL: 10 * time.Second,
	}

	redisCache, err := cache.NewRedisCache(config, coldStorage, nil)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Ключ отсутствует в Redis, но есть в Cold Storage
	value, err := redisCache.Get(ctx, "cold:key1")
	require.NoError(t, err)
	assert.Equal(t, []byte("cold value"), value)

	// Теперь ключ должен быть в Redis (Read-Through)
	time.Sleep(100 * time.Millisecond) // Даём время для асинхронной загрузки

	// Проверяем что теперь получается из Redis
	value2, err := redisCache.Get(ctx, "cold:key1")
	require.NoError(t, err)
	assert.Equal(t, []byte("cold value"), value2)
}

func TestRedisCache_WriteBehind(t *testing.T) {
	coldStorage := NewMockColdStorage()

	config := &cache.CacheConfig{
		RedisURL:             "localhost:6379",
		DefaultTTL:           10 * time.Second,
		WriteBehindEnabled:   true,
		WriteBehindInterval:  100 * time.Millisecond,
		WriteBehindBatchSize: 2,
	}

	redisCache, err := cache.NewRedisCache(config, coldStorage, nil)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Записываем несколько ключей
	err = redisCache.Set(ctx, "wb:key1", []byte("value1"), 5*time.Second)
	require.NoError(t, err)

	err = redisCache.Set(ctx, "wb:key2", []byte("value2"), 5*time.Second)
	require.NoError(t, err)

	// Ждём Write-Behind
	time.Sleep(200 * time.Millisecond)

	// Проверяем что данные попали в Cold Storage
	val1, err := coldStorage.Load(ctx, "wb:key1")
	require.NoError(t, err)
	assert.Equal(t, []byte("value1"), val1)

	val2, err := coldStorage.Load(ctx, "wb:key2")
	require.NoError(t, err)
	assert.Equal(t, []byte("value2"), val2)
}

func TestRedisCache_Metrics(t *testing.T) {
	config := &cache.CacheConfig{
		RedisURL:       "localhost:6379",
		DefaultTTL:     10 * time.Second,
		MetricsEnabled: true,
	}

	redisCache, err := cache.NewRedisCache(config, nil, nil)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Выполняем операции для генерации метрик
	redisCache.Set(ctx, "metrics:key1", []byte("value1"), 5*time.Second)
	redisCache.Get(ctx, "metrics:key1")    // hit
	redisCache.Get(ctx, "metrics:missing") // miss

	metrics := redisCache.GetMetrics()
	require.NotNil(t, metrics)

	assert.Greater(t, metrics.TotalRequests, int64(0))
	assert.Greater(t, metrics.CacheHits, int64(0))
	assert.Greater(t, metrics.CacheMisses, int64(0))
	assert.Greater(t, metrics.HitRatio, 0.0)
	assert.Less(t, metrics.HitRatio, 1.0)
}

func TestRedisCache_Invalidation(t *testing.T) {
	invalidator := NewMockInvalidator()

	config := &cache.CacheConfig{
		RedisURL:   "localhost:6379",
		DefaultTTL: 10 * time.Second,
	}

	redisCache, err := cache.NewRedisCache(config, nil, invalidator)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Устанавливаем значение
	err = redisCache.Set(ctx, "inv:key1", []byte("value1"), 5*time.Second)
	require.NoError(t, err)

	// Инвалидируем
	err = redisCache.Invalidate(ctx, "inv:key1")
	require.NoError(t, err)

	// Проверяем что ключ удалён
	exists, err := redisCache.Exists(ctx, "inv:key1")
	require.NoError(t, err)
	assert.False(t, exists)

	// Ждём завершения горутины invalidation
	time.Sleep(100 * time.Millisecond)

	// Проверяем что уведомление отправлено
	published := invalidator.GetPublished()
	assert.Contains(t, published, "inv:key1")
}

func TestNATSInvalidator_PubSub(t *testing.T) {
	// Пропускаем если NATS недоступен
	config := &cache.InvalidatorConfig{
		NATSURL: "localhost:4223",
		Subject: "test.cache.invalidation",
	}

	invalidator1, err := cache.NewNATSInvalidator(config, "node1")
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
		return
	}
	defer invalidator1.Close()

	invalidator2, err := cache.NewNATSInvalidator(config, "node2")
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
		return
	}
	defer invalidator2.Close()

	ctx := context.Background()

	// Подписываемся на инвалидации
	receivedKeys := make([]string, 0)
	var keysMutex sync.Mutex
	handler := func(key string) error {
		keysMutex.Lock()
		receivedKeys = append(receivedKeys, key)
		keysMutex.Unlock()
		return nil
	}

	err = invalidator2.SubscribeInvalidations(ctx, handler)
	require.NoError(t, err)

	// Ждём установки подписки
	time.Sleep(100 * time.Millisecond)

	// Публикуем инвалидацию
	err = invalidator1.PublishInvalidation(ctx, "test:key1")
	require.NoError(t, err)

	// Ждём получения сообщения
	time.Sleep(500 * time.Millisecond)

	// Проверяем что сообщение получено
	keysMutex.Lock()
	assert.Contains(t, receivedKeys, "test:key1")
	keysMutex.Unlock()

	// Проверяем метрики
	metrics1 := invalidator1.GetMetrics()
	assert.Greater(t, metrics1["published_count"], int64(0))

	metrics2 := invalidator2.GetMetrics()
	assert.Greater(t, metrics2["received_count"], int64(0))
}

func TestNATSInvalidator_Deduplication(t *testing.T) {
	config := &cache.InvalidatorConfig{
		NATSURL:      "localhost:4223",
		Subject:      "test.cache.dedup",
		DedupeWindow: 1 * time.Second,
	}

	invalidator, err := cache.NewNATSInvalidator(config, "node1")
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
		return
	}
	defer invalidator.Close()

	ctx := context.Background()

	// Публикуем один и тот же ключ несколько раз
	err = invalidator.PublishInvalidation(ctx, "dedup:key1")
	require.NoError(t, err)

	err = invalidator.PublishInvalidation(ctx, "dedup:key1")
	require.NoError(t, err)

	err = invalidator.PublishInvalidation(ctx, "dedup:key1")
	require.NoError(t, err)

	// Метрики должны показать только одну публикацию
	metrics := invalidator.GetMetrics()
	assert.Equal(t, int64(1), metrics["published_count"])
}

// BenchmarkRedisCache_Get измеряет производительность чтения из кеша.
func BenchmarkRedisCache_Get(b *testing.B) {
	config := &cache.CacheConfig{
		RedisURL:   "localhost:6379",
		DefaultTTL: 10 * time.Second,
	}

	redisCache, err := cache.NewRedisCache(config, nil, nil)
	if err != nil {
		b.Skipf("Redis not available, skipping benchmark: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Предварительно заполняем кеш
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench:key%d", i)
		value := []byte(fmt.Sprintf("value%d", i))
		redisCache.Set(ctx, key, value, 10*time.Second)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench:key%d", i%1000)
			_, err := redisCache.Get(ctx, key)
			if err != nil {
				b.Errorf("Get failed: %v", err)
			}
			i++
		}
	})
}

// BenchmarkRedisCache_Set измеряет производительность записи в кеш.
func BenchmarkRedisCache_Set(b *testing.B) {
	config := &cache.CacheConfig{
		RedisURL:   "localhost:6379",
		DefaultTTL: 10 * time.Second,
	}

	redisCache, err := cache.NewRedisCache(config, nil, nil)
	if err != nil {
		b.Skipf("Redis not available, skipping benchmark: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench:set:key%d", i)
			value := []byte(fmt.Sprintf("value%d", i))
			err := redisCache.Set(ctx, key, value, 10*time.Second)
			if err != nil {
				b.Errorf("Set failed: %v", err)
			}
			i++
		}
	})
}

// BenchmarkRedisCache_BatchGet измеряет производительность batch операций.
func BenchmarkRedisCache_BatchGet(b *testing.B) {
	config := &cache.CacheConfig{
		RedisURL:   "localhost:6379",
		DefaultTTL: 10 * time.Second,
	}

	redisCache, err := cache.NewRedisCache(config, nil, nil)
	if err != nil {
		b.Skipf("Redis not available, skipping benchmark: %v", err)
		return
	}
	defer redisCache.Close()

	ctx := context.Background()

	// Предварительно заполняем кеш
	items := make(map[string][]byte)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("batch:key%d", i)
		value := []byte(fmt.Sprintf("value%d", i))
		items[key] = value
	}
	redisCache.BatchSet(ctx, items, 10*time.Second)

	// Подготавливаем ключи для batch get
	keys := make([]string, 10)
	for i := 0; i < 10; i++ {
		keys[i] = fmt.Sprintf("batch:key%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := redisCache.BatchGet(ctx, keys)
		if err != nil {
			b.Errorf("BatchGet failed: %v", err)
		}
	}
}
