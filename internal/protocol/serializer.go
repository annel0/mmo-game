package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"
)

// MessageSerializer предоставляет методы сериализации и десериализации сообщений
type MessageSerializer struct {
	compressor   *zstd.Encoder
	decompressor *zstd.Decoder
}

// NewMessageSerializer создаёт новый сериализатор сообщений
func NewMessageSerializer() (*MessageSerializer, error) {
	// Создаём компрессор ZSTD с оптимальными настройками для игр
	compressor, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedDefault), // Баланс скорости и сжатия
		zstd.WithEncoderConcurrency(1),           // Меньше потоков для низкой латентности
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}

	// Создаём декомпрессор
	decompressor, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(1), // Меньше потоков для низкой латентности
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create decompressor: %w", err)
	}

	return &MessageSerializer{
		compressor:   compressor,
		decompressor: decompressor,
	}, nil
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

// Serialize сериализует NetGameMessage в байты
func (ms *MessageSerializer) Serialize(msg *NetGameMessage) ([]byte, error) {
	// Сериализуем protobuf
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Применяем сжатие если указано
	if msg.Compression == CompressionType_ZSTD {
		compressed := ms.compressor.EncodeAll(data, nil)
		data = compressed
	}

	// Добавляем заголовок с длиной сообщения (4 байта little-endian)
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(len(data)))

	// Возвращаем заголовок + данные
	result := make([]byte, 0, len(header)+len(data))
	result = append(result, header...)
	result = append(result, data...)

	return result, nil
}

// Deserialize десериализует байты в NetGameMessage
func (ms *MessageSerializer) Deserialize(data []byte) (*NetGameMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("message too short: %d bytes", len(data))
	}

	// Читаем длину из заголовка
	length := binary.LittleEndian.Uint32(data[:4])

	// Проверяем, что длина соответствует данным
	if uint32(len(data)-4) != length {
		return nil, fmt.Errorf("message length mismatch: expected %d, got %d",
			length, len(data)-4)
	}

	payload := data[4:]

	// Пытаемся десериализовать, чтобы узнать тип сжатия
	// Делаем это дважды из-за того, что нужно знать compression type
	var tempMsg NetGameMessage
	if err := proto.Unmarshal(payload, &tempMsg); err == nil {
		// Если сжатие применялось, декомпрессируем
		if tempMsg.Compression == CompressionType_ZSTD {
			decompressed, err := ms.decompressor.DecodeAll(payload, nil)
			if err != nil {
				return nil, fmt.Errorf("decompression failed: %w", err)
			}
			payload = decompressed
		}
	}

	// Окончательная десериализация
	var msg NetGameMessage
	if err := proto.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// SerializeBatch сериализует несколько сообщений в один буфер
// Полезно для пакетной отправки
func (ms *MessageSerializer) SerializeBatch(messages []*NetGameMessage) ([]byte, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages to serialize")
	}

	var totalSize int
	serializedMessages := make([][]byte, len(messages))

	// Сериализуем каждое сообщение
	for i, msg := range messages {
		data, err := ms.Serialize(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize message %d: %w", i, err)
		}
		serializedMessages[i] = data
		totalSize += len(data)
	}

	// Создаём батч с заголовком: [count (4 bytes)][message1][message2]...
	result := make([]byte, 0, 4+totalSize)

	// Записываем количество сообщений
	countBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(countBytes, uint32(len(messages)))
	result = append(result, countBytes...)

	// Добавляем сериализованные сообщения
	for _, data := range serializedMessages {
		result = append(result, data...)
	}

	return result, nil
}

// DeserializeBatch десериализует батч сообщений
func (ms *MessageSerializer) DeserializeBatch(data []byte) ([]*NetGameMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("batch too short: %d bytes", len(data))
	}

	// Читаем количество сообщений
	count := binary.LittleEndian.Uint32(data[:4])
	if count == 0 {
		return nil, fmt.Errorf("invalid message count: 0")
	}
	if count > 1000 { // Разумное ограничение
		return nil, fmt.Errorf("too many messages in batch: %d", count)
	}

	messages := make([]*NetGameMessage, 0, count)
	offset := 4

	// Десериализуем каждое сообщение
	for i := uint32(0); i < count; i++ {
		if offset >= len(data) {
			return nil, fmt.Errorf("unexpected end of batch at message %d", i)
		}

		// Проверяем, что у нас есть хотя бы заголовок
		if offset+4 > len(data) {
			return nil, fmt.Errorf("incomplete message header at message %d", i)
		}

		// Читаем длину сообщения
		msgLength := binary.LittleEndian.Uint32(data[offset : offset+4])
		totalMsgLength := 4 + int(msgLength)

		// Проверяем, что у нас есть все данные сообщения
		if offset+totalMsgLength > len(data) {
			return nil, fmt.Errorf("incomplete message data at message %d", i)
		}

		// Десериализуем сообщение
		msgData := data[offset : offset+totalMsgLength]
		msg, err := ms.Deserialize(msgData)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize message %d: %w", i, err)
		}

		messages = append(messages, msg)
		offset += totalMsgLength
	}

	return messages, nil
}

// Close освобождает ресурсы сериализатора
func (ms *MessageSerializer) Close() error {
	if ms.compressor != nil {
		ms.compressor.Close()
	}
	if ms.decompressor != nil {
		ms.decompressor.Close()
	}
	return nil
}

// GetCompressionRatio возвращает примерный коэффициент сжатия для сообщения
func (ms *MessageSerializer) GetCompressionRatio(msg *NetGameMessage) (float64, error) {
	// Сериализуем без сжатия
	originalCompression := msg.Compression
	msg.Compression = CompressionType_NONE

	uncompressed, err := proto.Marshal(msg)
	if err != nil {
		msg.Compression = originalCompression
		return 0, err
	}

	// Сжимаем
	compressed := ms.compressor.EncodeAll(uncompressed, nil)

	// Восстанавливаем оригинальное значение
	msg.Compression = originalCompression

	if len(uncompressed) == 0 {
		return 1.0, nil
	}

	return float64(len(compressed)) / float64(len(uncompressed)), nil
}

// ShouldCompress определяет, стоит ли сжимать сообщение
// Возвращает true если сжатие может дать значительную экономию
func (ms *MessageSerializer) ShouldCompress(msg *NetGameMessage) bool {
	// Небольшие сообщения (< 100 байт) обычно не стоит сжимать
	data, err := proto.Marshal(msg)
	if err != nil || len(data) < 100 {
		return false
	}

	// Для некоторых типов сообщений сжатие более эффективно
	switch msg.Payload.(type) {
	case *NetGameMessage_ChunkData:
		return true // Данные чанков хорошо сжимаются
	case *NetGameMessage_ChatBroadcast:
		return true // Текстовые сообщения хорошо сжимаются
	case *NetGameMessage_WorldEvent:
		return true // События мира могут быть большими
	case *NetGameMessage_Ping, *NetGameMessage_Pong:
		return false // Пинги должны быть быстрыми
	case *NetGameMessage_EntityMove:
		return false // Движение - частые небольшие сообщения
	default:
		// Для остальных проверяем размер
		return len(data) > 200
	}
}
