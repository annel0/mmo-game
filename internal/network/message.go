package network

import (
	"encoding/json"
	"fmt"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/annel0/mmo-game/internal/world/block"
)

// ===== Аутентификация =====

// JSONAuthRequest представляет запрос на аутентификацию
type JSONAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // В реальном приложении лучше использовать токены или хеши
}

// JSONAuthResponse представляет ответ на запрос аутентификации
type JSONAuthResponse struct {
	Success  bool   `json:"success"`
	PlayerID uint64 `json:"player_id"`
	Token    string `json:"token"` // JWT токен для последующих запросов
	Message  string `json:"message,omitempty"`
}

// ===== Данные чанка =====

// JSONChunkDataResponse представляет информацию о чанке
type JSONChunkDataResponse struct {
	ChunkX   int               `json:"chunk_x"`
	ChunkY   int               `json:"chunk_y"`
	Blocks   [][]block.BlockID `json:"blocks"`
	Entities []JSONEntityData  `json:"entities,omitempty"`
	Metadata map[string][]byte `json:"metadata,omitempty"` // Сериализованные метаданные блоков
}

// JSONEntityData представляет данные сущности
type JSONEntityData struct {
	ID       uint64                 `json:"id"`
	Type     uint16                 `json:"type"`
	Position vec.Vec2               `json:"position"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ===== Обновление блока =====

// JSONBlockUpdateRequest представляет запрос на изменение блока
type JSONBlockUpdateRequest struct {
	X       int                    `json:"x"`
	Y       int                    `json:"y"`
	BlockID block.BlockID          `json:"block_id"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// JSONBlockUpdateResponse представляет ответ на запрос изменения блока
type JSONBlockUpdateResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
	BlockID block.BlockID          `json:"block_id,omitempty"`
	X       int                    `json:"x,omitempty"`
	Y       int                    `json:"y,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	Effects []string               `json:"effects,omitempty"`
}

// ===== Действия сущности =====

// JSONEntityActionType определяет тип действия сущности
type JSONEntityActionType uint8

const (
	JSONEntityActionInteract JSONEntityActionType = iota
	JSONEntityActionAttack
	JSONEntityActionUseItem
	JSONEntityActionPickup
)

// JSONEntityActionRequest представляет запрос на действие сущности
type JSONEntityActionRequest struct {
	ActionType JSONEntityActionType   `json:"action_type"`
	TargetID   uint64                 `json:"target_id,omitempty"` // ID цели (если есть)
	Position   *vec.Vec2              `json:"position,omitempty"`  // Позиция действия (если есть)
	ItemID     uint32                 `json:"item_id,omitempty"`   // ID предмета (если действие с предметом)
	Params     map[string]interface{} `json:"params,omitempty"`    // Дополнительные параметры
}

// JSONEntityActionResponse представляет ответ на запрос действия сущности
type JSONEntityActionResponse struct {
	Success bool                   `json:"success"`
	Results map[string]interface{} `json:"results,omitempty"`
	Message string                 `json:"message,omitempty"`
}

// ===== Перемещение сущности =====

// JSONEntityMoveRequest представляет запрос на перемещение сущности
type JSONEntityMoveRequest struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Running bool `json:"running,omitempty"` // Бег или ходьба
}

// JSONEntityMoveResponse представляет ответ на запрос перемещения сущности
type JSONEntityMoveResponse struct {
	Success bool   `json:"success"`
	X       int    `json:"x"` // Фактическая X позиция после перемещения
	Y       int    `json:"y"` // Фактическая Y позиция после перемещения
	Message string `json:"message,omitempty"`
}

// ===== Сообщения чата =====

// JSONChatMessageType определяет тип сообщения чата
type JSONChatMessageType uint8

const (
	JSONChatGlobal  JSONChatMessageType = iota // Глобальный чат
	JSONChatLocal                              // Локальный чат (в радиусе видимости)
	JSONChatPrivate                            // Личное сообщение
	JSONChatSystem                             // Системное сообщение
)

// JSONChatRequest представляет запрос на отправку сообщения в чат
type JSONChatRequest struct {
	Type     JSONChatMessageType `json:"type"`
	Message  string              `json:"message"`
	TargetID uint64              `json:"target_id,omitempty"` // Для личных сообщений
}

// JSONChatResponse представляет сообщение чата
type JSONChatResponse struct {
	Type       JSONChatMessageType `json:"type"`
	Message    string              `json:"message"`
	SenderID   uint64              `json:"sender_id"`
	SenderName string              `json:"sender_name"`
	Timestamp  int64               `json:"timestamp"`
}

// ===== Функции сериализации/десериализации =====

// SerializeMessage сериализует структуру в JSON
func SerializeMessage(msg interface{}) ([]byte, error) {
	return json.Marshal(msg)
}

// DeserializeJSONAuth десериализует запрос аутентификации
func DeserializeJSONAuth(data []byte) (*JSONAuthRequest, error) {
	var req JSONAuthRequest
	err := json.Unmarshal(data, &req)
	if err != nil {
		return nil, fmt.Errorf("ошибка десериализации запроса аутентификации: %w", err)
	}
	return &req, nil
}

// DeserializeJSONBlockUpdate десериализует запрос обновления блока
func DeserializeJSONBlockUpdate(data []byte) (*JSONBlockUpdateRequest, error) {
	var req JSONBlockUpdateRequest
	err := json.Unmarshal(data, &req)
	if err != nil {
		return nil, fmt.Errorf("ошибка десериализации запроса обновления блока: %w", err)
	}
	return &req, nil
}

// DeserializeJSONEntityAction десериализует запрос действия сущности
func DeserializeJSONEntityAction(data []byte) (*JSONEntityActionRequest, error) {
	var req JSONEntityActionRequest
	err := json.Unmarshal(data, &req)
	if err != nil {
		return nil, fmt.Errorf("ошибка десериализации запроса действия сущности: %w", err)
	}
	return &req, nil
}

// DeserializeJSONEntityMove десериализует запрос перемещения сущности
func DeserializeJSONEntityMove(data []byte) (*JSONEntityMoveRequest, error) {
	var req JSONEntityMoveRequest
	err := json.Unmarshal(data, &req)
	if err != nil {
		return nil, fmt.Errorf("ошибка десериализации запроса перемещения сущности: %w", err)
	}
	return &req, nil
}

// DeserializeJSONChat десериализует запрос чата
func DeserializeJSONChat(data []byte) (*JSONChatRequest, error) {
	var req JSONChatRequest
	err := json.Unmarshal(data, &req)
	if err != nil {
		return nil, fmt.Errorf("ошибка десериализации запроса чата: %w", err)
	}
	return &req, nil
}

// ConvertChunkToJSON преобразует чанк из внутреннего представления в формат JSON-ответа
func ConvertChunkToJSON(chunk *world.Chunk) *JSONChunkDataResponse {
	resp := &JSONChunkDataResponse{
		ChunkX:   chunk.Coords.X,
		ChunkY:   chunk.Coords.Y,
		Blocks:   make([][]block.BlockID, 16),
		Metadata: make(map[string][]byte),
	}

	// Копируем блоки
	chunk.Mu.RLock()
	for x := 0; x < 16; x++ {
		resp.Blocks[x] = make([]block.BlockID, 16)
		for y := 0; y < 16; y++ {
			resp.Blocks[x][y] = chunk.Blocks[x][y]
		}
	}

	// Сериализуем метаданные
	for pos, meta := range chunk.Metadata {
		key := fmt.Sprintf("%d:%d", pos.X, pos.Y)
		metaBytes, err := json.Marshal(meta)
		if err == nil {
			resp.Metadata[key] = metaBytes
		}
	}
	chunk.Mu.RUnlock()

	return resp
}
