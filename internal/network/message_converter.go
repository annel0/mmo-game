package network

import (
	"fmt"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
)

// MessageConverter предоставляет утилиты для конвертации между GameMessage и NetGameMessage
type MessageConverter struct {
	serializer *protocol.MessageSerializer
}

// NewMessageConverter создаёт новый конвертер сообщений
func NewMessageConverter() (*MessageConverter, error) {
	serializer, err := protocol.NewMessageSerializer()
	if err != nil {
		return nil, fmt.Errorf("failed to create serializer: %w", err)
	}

	return &MessageConverter{
		serializer: serializer,
	}, nil
}

// GameToNet конвертирует GameMessage в NetGameMessage
func (mc *MessageConverter) GameToNet(gameMsg *protocol.GameMessage) (*protocol.NetGameMessage, error) {
	netMsg := &protocol.NetGameMessage{
		Sequence: gameMsg.Sequence,
		Flags:    protocol.NetFlags_RELIABLE_ORDERED, // По умолчанию
	}

	if gameMsg.Ack != nil {
		netMsg.Ack = *gameMsg.Ack
	}
	if gameMsg.AckBits != nil {
		netMsg.AckBits = *gameMsg.AckBits
	}

	// Десериализуем payload и определяем тип
	tempNetMsg, err := mc.serializer.Deserialize(gameMsg.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize payload: %w", err)
	}

	netMsg.Payload = tempNetMsg.Payload
	return netMsg, nil
}

// NetToGame конвертирует NetGameMessage в GameMessage
func (mc *MessageConverter) NetToGame(netMsg *protocol.NetGameMessage) (*protocol.GameMessage, error) {
	// Создаём GameMessage из NetGameMessage
	gameMsg := &protocol.GameMessage{
		Type:      protocol.MessageType_UNKNOWN, // Будет определён ниже
		Timestamp: time.Now().UnixNano(),
		Sequence:  netMsg.Sequence,
		Ack:       &netMsg.Ack,
		AckBits:   &netMsg.AckBits,
	}

	// Определяем тип и сериализуем payload
	var err error
	switch payload := netMsg.Payload.(type) {
	case *protocol.NetGameMessage_AuthRequest:
		gameMsg.Type = protocol.MessageType_AUTH
		gameMsg.Payload, err = mc.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_ChunkRequest:
		gameMsg.Type = protocol.MessageType_CHUNK_REQUEST
		gameMsg.Payload, err = mc.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_BlockUpdate:
		gameMsg.Type = protocol.MessageType_BLOCK_UPDATE
		gameMsg.Payload, err = mc.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_EntityMove:
		gameMsg.Type = protocol.MessageType_ENTITY_MOVE
		gameMsg.Payload, err = mc.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_Chat:
		gameMsg.Type = protocol.MessageType_CHAT
		gameMsg.Payload, err = mc.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_Ping:
		gameMsg.Type = protocol.MessageType_PING
		gameMsg.Payload, err = mc.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_Pong:
		gameMsg.Type = protocol.MessageType_PING
		gameMsg.Payload, err = mc.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	default:
		return nil, fmt.Errorf("unsupported message type: %T", payload)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to serialize payload: %w", err)
	}

	return gameMsg, nil
}

// GetSendOptions определяет опции отправки для GameMessage
func (mc *MessageConverter) GetSendOptions(gameMsg *protocol.GameMessage) *SendOptions {
	opts := &SendOptions{
		Priority: PriorityNormal,
		Flags:    protocol.NetFlags_RELIABLE_ORDERED,
	}

	// Настраиваем опции в зависимости от типа сообщения
	switch gameMsg.Type {
	case protocol.MessageType_PING:
		opts.Priority = PriorityCritical
		opts.Flags = protocol.NetFlags_UNRELIABLE_UNORDERED
	case protocol.MessageType_ENTITY_MOVE:
		opts.Priority = PriorityHigh
		opts.Flags = protocol.NetFlags_UNRELIABLE_UNORDERED
	case protocol.MessageType_CHUNK_DATA:
		opts.Compression = protocol.CompressionType_ZSTD
	case protocol.MessageType_CHAT:
		opts.Compression = protocol.CompressionType_ZSTD
	}

	return opts
}

// Close закрывает конвертер
func (mc *MessageConverter) Close() error {
	if mc.serializer != nil {
		return mc.serializer.Close()
	}
	return nil
}
