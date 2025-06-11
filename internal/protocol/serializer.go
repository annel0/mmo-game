package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
)

// MessageSerializer предоставляет функции для сериализации и десериализации сообщений
type MessageSerializer struct{}

// NewMessageSerializer создает новый сериализатор сообщений
func NewMessageSerializer() *MessageSerializer {
	return &MessageSerializer{}
}

// SerializeMessage сериализует сообщение в формат Protocol Buffers
func (ms *MessageSerializer) SerializeMessage(msgType MessageType, payload proto.Message) ([]byte, error) {
	// Сериализуем полезную нагрузку
	payloadData, err := proto.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации полезной нагрузки: %w", err)
	}

	// Создаем GameMessage в соответствии с proto-определением
	gameMessage := &GameMessage{
		Type:      msgType,
		Timestamp: time.Now().UnixNano(),
		Sequence:  0, // Для простых сообщений не используем sequence
		Payload:   payloadData,
	}

	// Сериализуем GameMessage с использованием proto
	messageData, err := proto.Marshal(gameMessage)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации сообщения: %w", err)
	}

	return messageData, nil
}

// DeserializeMessage десериализует данные в GameMessage
func (ms *MessageSerializer) DeserializeMessage(data []byte) (*GameMessage, error) {
	// Десериализуем в GameMessage из proto-определения
	protoMessage := &GameMessage{}
	if err := proto.Unmarshal(data, protoMessage); err != nil {
		return nil, fmt.Errorf("ошибка десериализации сообщения: %w", err)
	}

	return protoMessage, nil
}

// DeserializePayload десериализует полезную нагрузку сообщения в указанный тип
func (ms *MessageSerializer) DeserializePayload(msg *GameMessage, payload proto.Message) error {
	if err := proto.Unmarshal(msg.Payload, payload); err != nil {
		return fmt.Errorf("ошибка десериализации полезной нагрузки: %w", err)
	}
	return nil
}

// SerializeData сериализует структуру данных в бинарный формат
func (ms *MessageSerializer) SerializeData(data interface{}) ([]byte, error) {
	// Для сложных структур сначала преобразуем в JSON, затем в бинарный формат
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации в JSON: %w", err)
	}

	// Создаем JsonMetadata
	jsonMetadata := &JsonMetadata{
		JsonData: string(jsonData),
	}

	// Сериализуем в Protocol Buffers
	return proto.Marshal(jsonMetadata)
}

// DeserializeData десериализует бинарные данные в указанную структуру
func (ms *MessageSerializer) DeserializeData(data []byte, result interface{}) error {
	// Десериализуем из Protocol Buffers в JsonMetadata
	jsonMetadata := &JsonMetadata{}
	if err := proto.Unmarshal(data, jsonMetadata); err != nil {
		return fmt.Errorf("ошибка десериализации JsonMetadata: %w", err)
	}

	// Десериализуем из JSON в указанную структуру
	if err := json.Unmarshal([]byte(jsonMetadata.JsonData), result); err != nil {
		return fmt.Errorf("ошибка десериализации из JSON: %w", err)
	}

	return nil
}

// MapToJsonMetadata преобразует map в структуру JsonMetadata
func MapToJsonMetadata(metadata map[string]interface{}) (string, error) {
	jsonData, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

// JsonToMap преобразует строку JSON в map
func JsonToMap(jsonStr string) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Вспомогательные функции для работы с бинарными данными

// WriteUint32 записывает uint32 в big-endian формате
func WriteUint32(val uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, val)
	return b
}

// WriteUint64 записывает uint64 в big-endian формате
func WriteUint64(val uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, val)
	return b
}

// ReadUint32 читает uint32 из big-endian формата
func ReadUint32(data []byte) uint32 {
	return binary.BigEndian.Uint32(data)
}

// ReadUint64 читает uint64 из big-endian формата
func ReadUint64(data []byte) uint64 {
	return binary.BigEndian.Uint64(data)
}
