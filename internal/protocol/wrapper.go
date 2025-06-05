package protocol

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// MsgType определяет тип сообщения в системе
type MsgType int32

// Определение констант для типов сообщений
const (
	MsgUnknown      MsgType = 0
	MsgAuth         MsgType = 1
	MsgAuthResponse MsgType = 2
	MsgPing         MsgType = 3
	MsgPong         MsgType = 4
	MsgError        MsgType = 5

	// Регистрация
	MsgRegister MsgType = 9

	// Блоки и чанки
	MsgBlockUpdate         MsgType = 10
	MsgBlockUpdateResponse MsgType = 11
	MsgChunkRequest        MsgType = 12
	MsgChunkData           MsgType = 13

	// Сущности
	MsgEntitySpawn          MsgType = 20
	MsgEntityDespawn        MsgType = 21
	MsgEntityMove           MsgType = 22
	MsgEntityAction         MsgType = 23
	MsgEntityActionResponse MsgType = 24

	// Чат
	MsgChat          MsgType = 30
	MsgChatBroadcast MsgType = 31
)

// GameMsg представляет основное сообщение протокола
type GameMsg struct {
	Type    MsgType
	Payload []byte
}

// NewGameMsg создает новое игровое сообщение
func NewGameMsg(msgType MsgType, payload []byte) *GameMsg {
	return &GameMsg{
		Type:    msgType,
		Payload: payload,
	}
}

// MockProtoMsg представляет мок для proto.Message
type MockProtoMsg struct {
	Data interface{}
}

// Reset реализует proto.Message
func (m *MockProtoMsg) Reset() {}

// String реализует proto.Message
func (m *MockProtoMsg) String() string {
	return "MockProtoMsg"
}

// ProtoMessage реализует proto.Message
func (m *MockProtoMsg) ProtoMessage() {}

// ProtoReflect реализует protoreflect.ProtoMessage
func (m *MockProtoMsg) ProtoReflect() protoreflect.Message {
	return nil
}

// WrapMessage оборачивает любые данные в proto.Message
func WrapMessage(data interface{}) proto.Message {
	// Для вызовов, требующих proto.Message, но не использующих его
	// методы, используем заглушку для проверки типа
	return &MockProtoMsg{
		Data: data,
	}
}
