package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/go-redis/redis/v8"
)

// RedisPositionRepository хранит позиции игроков в Redis для быстрого доступа
type RedisPositionRepository struct {
	client      *redis.Client
	ctx         context.Context
	keyPrefix   string
	ttl         time.Duration
	batchSize   int
	batchMu     sync.Mutex
	batchBuffer map[string]*PlayerPosition
	batchTicker *time.Ticker
	shutdown    chan struct{}
	wg          sync.WaitGroup
}

// PlayerPosition представляет позицию игрока
type PlayerPosition struct {
	PlayerID  string        `json:"player_id"`
	Position  vec.Vec3      `json:"position"`
	Velocity  vec.Vec2Float `json:"velocity"`
	Direction int           `json:"direction"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// RedisConfig содержит настройки подключения к Redis
type RedisConfig struct {
	Addr         string        // Адрес Redis сервера
	Password     string        // Пароль (пустой если не требуется)
	DB           int           // Номер базы данных
	KeyPrefix    string        // Префикс для ключей
	TTL          time.Duration // Время жизни записей
	BatchSize    int           // Размер батча для записи
	BatchFlushMs int           // Интервал сброса батча в миллисекундах
}

// DefaultRedisConfig возвращает конфигурацию по умолчанию
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		KeyPrefix:    "mmo:pos:",
		TTL:          5 * time.Minute,
		BatchSize:    100,
		BatchFlushMs: 100,
	}
}

// NewRedisPositionRepository создаёт новый Redis репозиторий для позиций
func NewRedisPositionRepository(config *RedisConfig) (*RedisPositionRepository, error) {
	if config == nil {
		config = DefaultRedisConfig()
	}

	// Создаём клиент Redis
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	ctx := context.Background()

	// Проверяем подключение
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	repo := &RedisPositionRepository{
		client:      client,
		ctx:         ctx,
		keyPrefix:   config.KeyPrefix,
		ttl:         config.TTL,
		batchSize:   config.BatchSize,
		batchBuffer: make(map[string]*PlayerPosition),
		batchTicker: time.NewTicker(time.Duration(config.BatchFlushMs) * time.Millisecond),
		shutdown:    make(chan struct{}),
	}

	// Запускаем фоновую горутину для сброса батчей
	repo.wg.Add(1)
	go repo.batchFlusher()

	log.Printf("🔴 Connected to Redis at %s", config.Addr)
	return repo, nil
}

// SavePosition сохраняет позицию игрока
func (rpr *RedisPositionRepository) SavePosition(playerID string, position vec.Vec3, velocity vec.Vec2Float, direction int) error {
	pos := &PlayerPosition{
		PlayerID:  playerID,
		Position:  position,
		Velocity:  velocity,
		Direction: direction,
		UpdatedAt: time.Now(),
	}

	// Добавляем в батч-буфер
	rpr.batchMu.Lock()
	rpr.batchBuffer[playerID] = pos

	// Если буфер заполнен, сбрасываем немедленно
	if len(rpr.batchBuffer) >= rpr.batchSize {
		batch := rpr.batchBuffer
		rpr.batchBuffer = make(map[string]*PlayerPosition)
		rpr.batchMu.Unlock()

		return rpr.flushBatch(batch)
	}

	rpr.batchMu.Unlock()
	return nil
}

// GetPosition получает позицию игрока
func (rpr *RedisPositionRepository) GetPosition(playerID string) (*PlayerPosition, error) {
	key := rpr.keyPrefix + playerID

	data, err := rpr.client.Get(rpr.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Позиция не найдена
	} else if err != nil {
		return nil, fmt.Errorf("failed to get position: %w", err)
	}

	var pos PlayerPosition
	if err := json.Unmarshal([]byte(data), &pos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal position: %w", err)
	}

	return &pos, nil
}

// GetPositions получает позиции нескольких игроков
func (rpr *RedisPositionRepository) GetPositions(playerIDs []string) (map[string]*PlayerPosition, error) {
	if len(playerIDs) == 0 {
		return make(map[string]*PlayerPosition), nil
	}

	// Формируем ключи
	keys := make([]string, len(playerIDs))
	for i, id := range playerIDs {
		keys[i] = rpr.keyPrefix + id
	}

	// Получаем данные пайплайном
	pipe := rpr.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))

	for i, key := range keys {
		cmds[i] = pipe.Get(rpr.ctx, key)
	}

	_, err := pipe.Exec(rpr.ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	// Парсим результаты
	result := make(map[string]*PlayerPosition)
	for i, cmd := range cmds {
		data, err := cmd.Result()
		if err == redis.Nil {
			continue // Пропускаем отсутствующие
		} else if err != nil {
			log.Printf("⚠️ Failed to get position for %s: %v", playerIDs[i], err)
			continue
		}

		var pos PlayerPosition
		if err := json.Unmarshal([]byte(data), &pos); err != nil {
			log.Printf("⚠️ Failed to unmarshal position for %s: %v", playerIDs[i], err)
			continue
		}

		result[playerIDs[i]] = &pos
	}

	return result, nil
}

// GetNearbyPlayers получает игроков в заданном прямоугольнике
func (rpr *RedisPositionRepository) GetNearbyPlayers(minX, minZ, maxX, maxZ float64) ([]*PlayerPosition, error) {
	// Redis не поддерживает пространственные запросы напрямую
	// Используем Geo команды Redis или храним дополнительный индекс

	// Для простоты используем подход с получением всех позиций и фильтрацией
	// В продакшене лучше использовать Redis Geo или отдельный spatial index

	// Получаем все ключи позиций
	pattern := rpr.keyPrefix + "*"
	keys, err := rpr.client.Keys(rpr.ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to scan keys: %w", err)
	}

	if len(keys) == 0 {
		return []*PlayerPosition{}, nil
	}

	// Получаем все позиции пайплайном
	pipe := rpr.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))

	for i, key := range keys {
		cmds[i] = pipe.Get(rpr.ctx, key)
	}

	_, err = pipe.Exec(rpr.ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	// Фильтруем по координатам
	result := make([]*PlayerPosition, 0)
	for _, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			continue
		}

		var pos PlayerPosition
		if err := json.Unmarshal([]byte(data), &pos); err != nil {
			continue
		}

		// Проверяем попадание в прямоугольник
		if float64(pos.Position.X) >= minX && float64(pos.Position.X) <= maxX &&
			float64(pos.Position.Z) >= minZ && float64(pos.Position.Z) <= maxZ {
			result = append(result, &pos)
		}
	}

	return result, nil
}

// DeletePosition удаляет позицию игрока
func (rpr *RedisPositionRepository) DeletePosition(playerID string) error {
	key := rpr.keyPrefix + playerID

	// Удаляем из батч-буфера если есть
	rpr.batchMu.Lock()
	delete(rpr.batchBuffer, playerID)
	rpr.batchMu.Unlock()

	// Удаляем из Redis
	if err := rpr.client.Del(rpr.ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete position: %w", err)
	}

	return nil
}

// GetActivePlayerCount возвращает количество активных игроков
func (rpr *RedisPositionRepository) GetActivePlayerCount() (int64, error) {
	pattern := rpr.keyPrefix + "*"

	// Используем SCAN для подсчёта ключей
	var count int64
	iter := rpr.client.Scan(rpr.ctx, 0, pattern, 0).Iterator()

	for iter.Next(rpr.ctx) {
		count++
	}

	if err := iter.Err(); err != nil {
		return 0, fmt.Errorf("failed to count players: %w", err)
	}

	return count, nil
}

// SavePositionsGeo сохраняет позиции используя Redis GEO для пространственных запросов
func (rpr *RedisPositionRepository) SavePositionGeo(playerID string, position vec.Vec3) error {
	geoKey := rpr.keyPrefix + "geo"

	// Сохраняем в GEO индекс (X как longitude, Z как latitude)
	// Нормализуем координаты в диапазон долготы/широты
	lon := float64(position.X) / 1000.0 // Предполагаем мир 1000x1000
	lat := float64(position.Z) / 1000.0

	// Ограничиваем значения
	if lon < -180 {
		lon = -180
	} else if lon > 180 {
		lon = 180
	}

	if lat < -90 {
		lat = -90
	} else if lat > 90 {
		lat = 90
	}

	// GeoAdd ожидает пару координат
	if err := rpr.client.GeoAdd(rpr.ctx, geoKey, &redis.GeoLocation{
		Name:      playerID,
		Longitude: lon,
		Latitude:  lat,
	}).Err(); err != nil {
		return fmt.Errorf("failed to save geo position: %w", err)
	}

	// Устанавливаем TTL для GEO ключа
	rpr.client.Expire(rpr.ctx, geoKey, rpr.ttl)

	return nil
}

// GetNearbyPlayersGeo получает игроков в радиусе используя GEO индекс
func (rpr *RedisPositionRepository) GetNearbyPlayersGeo(centerX, centerZ float64, radiusMeters float64) ([]string, error) {
	geoKey := rpr.keyPrefix + "geo"

	// Нормализуем координаты
	lon := centerX / 1000.0
	lat := centerZ / 1000.0

	// Поиск в радиусе
	query := &redis.GeoSearchQuery{
		Longitude:  lon,
		Latitude:   lat,
		Radius:     radiusMeters,
		RadiusUnit: "m",
	}

	names, err := rpr.client.GeoSearch(rpr.ctx, geoKey, query).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to search nearby players: %w", err)
	}

	// Результат уже []string с именами (ID игроков)
	return names, nil
}

// Close закрывает соединение с Redis
func (rpr *RedisPositionRepository) Close() error {
	// Останавливаем батч-флашер
	close(rpr.shutdown)
	rpr.wg.Wait()

	// Сбрасываем оставшиеся данные
	rpr.batchMu.Lock()
	if len(rpr.batchBuffer) > 0 {
		rpr.flushBatch(rpr.batchBuffer)
	}
	rpr.batchMu.Unlock()

	// Закрываем соединение
	return rpr.client.Close()
}

// Внутренние методы

// batchFlusher периодически сбрасывает батч-буфер
func (rpr *RedisPositionRepository) batchFlusher() {
	defer rpr.wg.Done()

	for {
		select {
		case <-rpr.shutdown:
			return
		case <-rpr.batchTicker.C:
			rpr.batchMu.Lock()
			if len(rpr.batchBuffer) > 0 {
				batch := rpr.batchBuffer
				rpr.batchBuffer = make(map[string]*PlayerPosition)
				rpr.batchMu.Unlock()

				if err := rpr.flushBatch(batch); err != nil {
					log.Printf("❌ Failed to flush batch: %v", err)
				}
			} else {
				rpr.batchMu.Unlock()
			}
		}
	}
}

// flushBatch записывает батч позиций в Redis
func (rpr *RedisPositionRepository) flushBatch(batch map[string]*PlayerPosition) error {
	if len(batch) == 0 {
		return nil
	}

	pipe := rpr.client.Pipeline()

	for playerID, pos := range batch {
		key := rpr.keyPrefix + playerID

		data, err := json.Marshal(pos)
		if err != nil {
			log.Printf("⚠️ Failed to marshal position for %s: %v", playerID, err)
			continue
		}

		pipe.Set(rpr.ctx, key, data, rpr.ttl)

		// Также обновляем GEO индекс
		if err := rpr.SavePositionGeo(playerID, pos.Position); err != nil {
			log.Printf("⚠️ Failed to update GEO for %s: %v", playerID, err)
		}
	}

	_, err := pipe.Exec(rpr.ctx)
	if err != nil {
		return fmt.Errorf("failed to execute batch: %w", err)
	}

	return nil
}

// GetStats возвращает статистику репозитория
func (rpr *RedisPositionRepository) GetStats() (string, error) {
	info, err := rpr.client.Info(rpr.ctx, "stats").Result()
	if err != nil {
		return "", err
	}

	count, _ := rpr.GetActivePlayerCount()

	return fmt.Sprintf("Redis Position Repository: %d active players\n%s", count, info), nil
}
