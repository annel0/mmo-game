package network

import (
	"encoding/json"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// Константы типов сообщений
const (
	// Клиент -> Сервер
	MsgTypeAuth            = "auth"            // Аутентификация
	MsgTypeMovement        = "movement"        // Запрос на перемещение
	MsgTypeAction          = "action"          // Действие (атака, использование предмета и т.д.)
	MsgTypeChat            = "chat"            // Сообщение чата
	MsgTypeBlockInteract   = "block_interact"  // Взаимодействие с блоком
	MsgTypeEntityInteract  = "entity_interact" // Взаимодействие с сущностью
	MsgTypeInventoryAction = "inventory"       // Действие с инвентарем

	// Сервер -> Клиент
	MsgTypeAuthResponse    = "auth_response"    // Ответ на аутентификацию
	MsgTypeWorldChunk      = "world_chunk"      // Данные чанка мира
	MsgTypeEntityUpdate    = "entity_update"    // Обновление сущности
	MsgTypeEntitySpawn     = "entity_spawn"     // Создание сущности
	MsgTypeEntityDespawn   = "entity_despawn"   // Удаление сущности
	MsgTypeBlockUpdate     = "block_update"     // Изменение блока
	MsgTypeChatBroadcast   = "chat_broadcast"   // Широковещательное сообщение чата
	MsgTypePlayerInventory = "player_inventory" // Содержимое инвентаря игрока
	MsgTypeGameEvent       = "game_event"       // Игровое событие
	MsgTypeServerMessage   = "server_message"   // Системное сообщение

	// Добавим новые типы сообщений
	MsgTypeWorldEvent    = "world_event"    // События мира (смена времени суток, погода и т.д.)
	MsgTypeChunkRequest  = "chunk_request"  // Запрос чанка от клиента
	MsgTypeChunkResponse = "chunk_response" // Ответ с данными чанка
	MsgTypePlayerStats   = "player_stats"   // Статистика игрока (здоровье, голод и т.д.)
	MsgTypeEntityAI      = "entity_ai"      // События ИИ сущностей
	MsgTypeQuestEvent    = "quest_event"    // События квестов
)

// Константы действий
const (
	ActionAttack     = "attack"      // Атака
	ActionUseItem    = "use_item"    // Использование предмета
	ActionPickup     = "pickup"      // Подбор предмета
	ActionDrop       = "drop"        // Выбрасывание предмета
	ActionInteract   = "interact"    // Взаимодействие
	ActionTrade      = "trade"       // Торговля
	ActionCraft      = "craft"       // Крафт
	ActionEmote      = "emote"       // Эмоция
	ActionRespawn    = "respawn"     // Возрождение
	ActionBuildPlace = "build_place" // Строительство
	ActionBuildBreak = "build_break" // Разрушение
)

// WorldEventType описывает типы событий мира
type WorldEventType string

const (
	WorldEventDayNightCycle WorldEventType = "day_night_cycle" // Смена дня и ночи
	WorldEventWeather       WorldEventType = "weather"         // Изменение погоды
	WorldEventSeason        WorldEventType = "season"          // Смена сезона
)

// Message представляет базовую структуру сетевого сообщения
type Message struct {
	Type      string          `json:"type"`               // Тип сообщения
	Timestamp int64           `json:"timestamp"`          // Временная метка
	Data      json.RawMessage `json:"data"`               // Данные сообщения (зависят от типа)
	ClientID  string          `json:"client_id"`          // Идентификатор клиента
	Sequence  uint32          `json:"sequence"`           // Порядковый номер сообщения
	Ack       uint32          `json:"ack,omitempty"`      // Подтверждение (опционально)
	AckBits   uint32          `json:"ack_bits,omitempty"` // Битовая маска подтверждений (опционально)
}

// NewMessage создает новое сообщение указанного типа
func NewMessage(msgType string, data interface{}) (*Message, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:      msgType,
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		Data:      dataBytes,
	}, nil
}

// Структуры данных для сообщений от клиента к серверу

// AuthRequest представляет запрос на авторизацию
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // Опционально
	Token    string `json:"token,omitempty"`    // Опционально
}

// MovementRequest представляет запрос на перемещение
type MovementRequest struct {
	Direction entity.MovementDirection `json:"direction"` // Направление движения
	Timestamp int64                    `json:"timestamp"` // Клиентская временная метка
}

// ActionRequest представляет запрос на выполнение действия
type ActionRequest struct {
	Action    string   `json:"action"`    // Тип действия
	Target    uint64   `json:"target"`    // ID цели (если есть)
	ItemID    string   `json:"item_id"`   // ID предмета (если используется)
	Position  vec.Vec2 `json:"position"`  // Позиция (если применимо)
	Direction int      `json:"direction"` // Направление (если применимо)
}

// ChatMessage представляет сообщение чата
type ChatMessage struct {
	Content  string `json:"content"`             // Содержимое сообщения
	Channel  string `json:"channel"`             // Канал чата (общий, локальный, торговый и т.д.)
	TargetID uint64 `json:"target_id,omitempty"` // ID целевого игрока для приватных сообщений
}

// BlockInteractRequest представляет запрос на взаимодействие с блоком
type BlockInteractRequest struct {
	Position vec.Vec2 `json:"position"`          // Позиция блока
	Action   string   `json:"action"`            // Тип действия (разрушить, использовать и т.д.)
	ItemID   string   `json:"item_id,omitempty"` // ID используемого предмета (если есть)
}

// EntityInteractRequest представляет запрос на взаимодействие с сущностью
type EntityInteractRequest struct {
	EntityID uint64 `json:"entity_id"` // ID сущности
	Action   string `json:"action"`    // Тип действия
}

// InventoryActionRequest представляет запрос на действие с инвентарем
type InventoryActionRequest struct {
	Action    string `json:"action"`              // Тип действия (переместить, разделить, и т.д.)
	SlotFrom  int    `json:"slot_from"`           // Исходный слот
	SlotTo    int    `json:"slot_to"`             // Целевой слот
	ItemID    string `json:"item_id"`             // ID предмета
	Quantity  int    `json:"quantity"`            // Количество
	Container string `json:"container,omitempty"` // Тип контейнера, если не основной инвентарь
}

// Структуры данных для сообщений от сервера к клиенту

// AuthResponse представляет ответ на запрос авторизации
type AuthResponse struct {
	Success   bool   `json:"success"`    // Успешность авторизации
	Message   string `json:"message"`    // Сообщение (причина отказа и т.д.)
	PlayerID  uint64 `json:"player_id"`  // ID игрока
	Token     string `json:"token"`      // Токен сессии
	WorldName string `json:"world_name"` // Название мира
}

// EntityData представляет данные о сущности
type EntityData struct {
	ID         uint64                 `json:"id"`                  // ID сущности
	Type       entity.EntityType      `json:"type"`                // Тип сущности
	Position   vec.Vec2               `json:"position"`            // Позиция
	Velocity   vec.Vec2Float          `json:"velocity"`            // Скорость
	Direction  int                    `json:"direction"`           // Направление
	Active     bool                   `json:"active"`              // Активность
	Attributes map[string]interface{} `json:"attributes"`          // Атрибуты сущности
	Animation  string                 `json:"animation,omitempty"` // Текущая анимация
	Effects    []EntityEffect         `json:"effects,omitempty"`   // Визуальные эффекты
}

// EntityUpdateMessage представляет обновление состояния сущности
type EntityUpdateMessage struct {
	Entities []EntityData `json:"entities"` // Данные о сущностях
}

// EntitySpawnMessage представляет создание новой сущности
type EntitySpawnMessage struct {
	Entity EntityData `json:"entity"` // Данные о сущности
}

// EntityDespawnMessage представляет удаление сущности
type EntityDespawnMessage struct {
	EntityID uint64 `json:"entity_id"`        // ID сущности
	Reason   string `json:"reason,omitempty"` // Причина удаления
}

// BlockData представляет данные о блоке
type BlockData struct {
	Position vec.Vec2               `json:"position"`           // Позиция блока
	BlockID  uint16                 `json:"block_id"`           // ID типа блока
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Метаданные (опционально)
	Effects  []BlockEffect          `json:"effects,omitempty"`  // Визуальные эффекты
}

// BlockUpdateMessage представляет обновление блока
type BlockUpdateMessage struct {
	Blocks []BlockData `json:"blocks"` // Данные о блоках
}

// WorldChunkMessage представляет данные о чанке мира
type WorldChunkMessage struct {
	ChunkX   int                    `json:"chunk_x"`            // Координата X чанка
	ChunkY   int                    `json:"chunk_y"`            // Координата Y чанка
	Blocks   [][]uint16             `json:"blocks"`             // Матрица блоков (упрощенно)
	Entities []EntityData           `json:"entities"`           // Сущности в чанке
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Метаданные чанка (опционально)
}

// ChatBroadcastMessage представляет широковещательное сообщение чата
type ChatBroadcastMessage struct {
	SenderID   uint64 `json:"sender_id"`   // ID отправителя
	SenderName string `json:"sender_name"` // Имя отправителя
	Content    string `json:"content"`     // Содержимое сообщения
	Channel    string `json:"channel"`     // Канал чата
	Timestamp  int64  `json:"timestamp"`   // Временная метка
}

// InventoryItem представляет предмет в инвентаре
type InventoryItem struct {
	ItemID     string                 `json:"item_id"`              // ID предмета
	Quantity   int                    `json:"quantity"`             // Количество
	Durability int                    `json:"durability,omitempty"` // Прочность (опционально)
	Attributes map[string]interface{} `json:"attributes,omitempty"` // Атрибуты (опционально)
}

// PlayerInventoryMessage представляет содержимое инвентаря игрока
type PlayerInventoryMessage struct {
	Slots        map[int]InventoryItem `json:"slots"`               // Слоты инвентаря
	EquippedSlot int                   `json:"equipped_slot"`       // Активный слот
	Container    string                `json:"container,omitempty"` // Тип контейнера
}

// GameEventMessage представляет игровое событие
type GameEventMessage struct {
	EventType  string                 `json:"event_type"`           // Тип события
	EntityID   uint64                 `json:"entity_id,omitempty"`  // ID связанной сущности (опционально)
	Position   vec.Vec2               `json:"position,omitempty"`   // Позиция события (опционально)
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Дополнительные параметры
}

// ServerMessage представляет системное сообщение от сервера
type ServerMessage struct {
	MessageType string `json:"message_type"`       // Тип сообщения (info, warning, error)
	Content     string `json:"content"`            // Содержимое сообщения
	Duration    int    `json:"duration,omitempty"` // Продолжительность отображения (мс, опционально)
}

// ChunkRequestMessage представляет запрос чанка от клиента
type ChunkRequestMessage struct {
	ChunkX int `json:"chunk_x"` // Координата X чанка
	ChunkY int `json:"chunk_y"` // Координата Y чанка
}

// WorldEventMessage представляет событие мира
type WorldEventMessage struct {
	EventType WorldEventType         `json:"event_type"` // Тип события
	Data      map[string]interface{} `json:"data"`       // Данные события
	Timestamp int64                  `json:"timestamp"`  // Временная метка
}

// PlayerStatsMessage представляет статистику игрока
type PlayerStatsMessage struct {
	Health        int            `json:"health"`                   // Текущее здоровье
	MaxHealth     int            `json:"max_health"`               // Максимальное здоровье
	Hunger        int            `json:"hunger,omitempty"`         // Голод (если есть)
	Thirst        int            `json:"thirst,omitempty"`         // Жажда (если есть)
	Experience    int            `json:"experience"`               // Опыт
	Level         int            `json:"level"`                    // Уровень
	StatusEffects []StatusEffect `json:"status_effects,omitempty"` // Эффекты статуса
}

// StatusEffect представляет эффект статуса
type StatusEffect struct {
	Type      string `json:"type"`      // Тип эффекта
	Duration  int    `json:"duration"`  // Длительность в тиках
	Intensity int    `json:"intensity"` // Интенсивность
}

// QuestEventMessage представляет событие квеста
type QuestEventMessage struct {
	QuestID    string                 `json:"quest_id"`   // ID квеста
	EventType  string                 `json:"event_type"` // Тип события (начало, завершение и т.д.)
	Progress   int                    `json:"progress"`   // Прогресс выполнения (в процентах)
	Parameters map[string]interface{} `json:"parameters"` // Дополнительные параметры
}

// BlockEffect представляет визуальный эффект блока
type BlockEffect struct {
	Type       string                 `json:"type"`                 // Тип эффекта (частицы, анимация и т.д.)
	Duration   int                    `json:"duration,omitempty"`   // Длительность в тиках
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Параметры эффекта
}

// EntityEffect представляет визуальный эффект сущности
type EntityEffect struct {
	Type       string                 `json:"type"`                 // Тип эффекта
	Duration   int                    `json:"duration,omitempty"`   // Длительность в тиках
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Параметры эффекта
}
