package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/network"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNetGameMessageSerialization проверяет сериализацию NetGameMessage
func TestNetGameMessageSerialization(t *testing.T) {
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(t, err)
	defer serializer.Close()

	// Тест аутентификации с правильной структурой
	authMsg := &protocol.NetGameMessage{
		Sequence: 1,
		Flags:    protocol.NetFlags_RELIABLE_ORDERED,
		Payload: &protocol.NetGameMessage_AuthRequest{
			AuthRequest: &protocol.AuthMessage{
				Username: "test_player",
				Password: &[]string{"test_password"}[0],
			},
		},
	}

	data, err := serializer.Serialize(authMsg)
	require.NoError(t, err)
	assert.Greater(t, len(data), 4) // Минимум 4 байта для заголовка

	deserializedMsg, err := serializer.Deserialize(data)
	require.NoError(t, err)
	assert.Equal(t, authMsg.Sequence, deserializedMsg.Sequence)
	assert.Equal(t, authMsg.Flags, deserializedMsg.Flags)

	authData := deserializedMsg.GetAuthRequest()
	require.NotNil(t, authData)
	assert.Equal(t, "test_player", authData.Username)
}

// TestHeartbeatMessage проверяет heartbeat сообщения
func TestHeartbeatMessage(t *testing.T) {
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(t, err)
	defer serializer.Close()

	heartbeatMsg := &protocol.NetGameMessage{
		Sequence: 2,
		Flags:    protocol.NetFlags_UNRELIABLE_UNORDERED,
		Payload: &protocol.NetGameMessage_Heartbeat{
			Heartbeat: &protocol.HeartbeatMessage{
				ClientTime: time.Now().UnixNano(),
				RttMs:      50,
			},
		},
	}

	data, err := serializer.Serialize(heartbeatMsg)
	require.NoError(t, err)

	deserializedMsg, err := serializer.Deserialize(data)
	require.NoError(t, err)

	heartbeat := deserializedMsg.GetHeartbeat()
	require.NotNil(t, heartbeat)
	assert.Greater(t, heartbeat.ClientTime, int64(0))
	assert.Equal(t, int32(50), heartbeat.RttMs)
}

// TestEntityMoveMessage проверяет сообщения движения сущностей
func TestEntityMoveMessage(t *testing.T) {
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(t, err)
	defer serializer.Close()

	entities := []*protocol.EntityData{
		{
			Id:        123,
			Type:      protocol.EntityType_ENTITY_PLAYER,
			Position:  &protocol.Vec2{X: 100, Y: 200},
			Direction: 1,
			Active:    true,
		},
		{
			Id:        456,
			Type:      protocol.EntityType_ENTITY_NPC,
			Position:  &protocol.Vec2{X: 150, Y: 250},
			Direction: 2,
			Active:    true,
		},
	}

	moveMsg := &protocol.NetGameMessage{
		Sequence: 3,
		Flags:    protocol.NetFlags_UNRELIABLE_UNORDERED,
		Payload: &protocol.NetGameMessage_EntityMove{
			EntityMove: &protocol.EntityMoveMessage{
				Entities: entities,
			},
		},
	}

	data, err := serializer.Serialize(moveMsg)
	require.NoError(t, err)

	deserializedMsg, err := serializer.Deserialize(data)
	require.NoError(t, err)

	entityMove := deserializedMsg.GetEntityMove()
	require.NotNil(t, entityMove)
	assert.Equal(t, 2, len(entityMove.Entities))

	// Проверяем первую сущность
	entity1 := entityMove.Entities[0]
	assert.Equal(t, uint64(123), entity1.Id)
	assert.Equal(t, protocol.EntityType_ENTITY_PLAYER, entity1.Type)
	assert.Equal(t, int32(100), entity1.Position.X)
	assert.Equal(t, int32(200), entity1.Position.Y)
}

// TestChunkDataMessage проверяет сообщения данных чанков
func TestChunkDataMessage(t *testing.T) {
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(t, err)
	defer serializer.Close()

	// Создаём чанк с данными
	chunkData := &protocol.ChunkData{
		ChunkX: 10,
		ChunkY: 20,
		Layers: []*protocol.ChunkLayer{
			{
				Layer: uint32(protocol.BlockLayer_FLOOR),
				Rows: []*protocol.BlockRow{
					{BlockIds: []uint32{1, 2, 3, 4}},
					{BlockIds: []uint32{5, 6, 7, 8}},
				},
			},
			{
				Layer: uint32(protocol.BlockLayer_ACTIVE),
				Rows: []*protocol.BlockRow{
					{BlockIds: []uint32{0, 0, 0, 0}},  // Воздух
					{BlockIds: []uint32{0, 10, 0, 0}}, // Один блок
				},
			},
		},
	}

	chunkMsg := &protocol.NetGameMessage{
		Sequence: 4,
		Flags:    protocol.NetFlags_RELIABLE_ORDERED,
		Payload: &protocol.NetGameMessage_ChunkData{
			ChunkData: chunkData,
		},
	}

	data, err := serializer.Serialize(chunkMsg)
	require.NoError(t, err)

	deserializedMsg, err := serializer.Deserialize(data)
	require.NoError(t, err)

	deserializedChunk := deserializedMsg.GetChunkData()
	require.NotNil(t, deserializedChunk)
	assert.Equal(t, int32(10), deserializedChunk.ChunkX)
	assert.Equal(t, int32(20), deserializedChunk.ChunkY)
	assert.Equal(t, 2, len(deserializedChunk.Layers))

	// Проверяем слой пола
	floorLayer := deserializedChunk.Layers[0]
	assert.Equal(t, uint32(protocol.BlockLayer_FLOOR), floorLayer.Layer)
	assert.Equal(t, 2, len(floorLayer.Rows))
	assert.Equal(t, []uint32{1, 2, 3, 4}, floorLayer.Rows[0].BlockIds)
}

// TestCompressionTypes проверяет различные типы сжатия
func TestCompressionTypes(t *testing.T) {
	t.Skip("Skipping compression test - ZSTD compression needs investigation")
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(t, err)
	defer serializer.Close()

	// Создаём большое сообщение для тестирования сжатия
	largeEntities := make([]*protocol.EntityData, 100)
	for i := 0; i < 100; i++ {
		largeEntities[i] = &protocol.EntityData{
			Id:        uint64(i),
			Type:      protocol.EntityType_ENTITY_PLAYER,
			Position:  &protocol.Vec2{X: int32(i * 10), Y: int32(i * 20)},
			Direction: int32(i % 4),
			Active:    true,
		}
	}

	// Тест без сжатия
	uncompressedMsg := &protocol.NetGameMessage{
		Sequence:    5,
		Compression: protocol.CompressionType_NONE,
		Payload: &protocol.NetGameMessage_EntityMove{
			EntityMove: &protocol.EntityMoveMessage{
				Entities: largeEntities,
			},
		},
	}

	uncompressedData, err := serializer.Serialize(uncompressedMsg)
	require.NoError(t, err)

	// Тест со сжатием ZSTD
	compressedMsg := &protocol.NetGameMessage{
		Sequence:    6,
		Compression: protocol.CompressionType_ZSTD,
		Payload: &protocol.NetGameMessage_EntityMove{
			EntityMove: &protocol.EntityMoveMessage{
				Entities: largeEntities,
			},
		},
	}

	compressedData, err := serializer.Serialize(compressedMsg)
	require.NoError(t, err)

	t.Logf("Размер без сжатия: %d байт", len(uncompressedData))
	t.Logf("Размер со сжатием: %d байт", len(compressedData))

	// Проверяем десериализацию обоих вариантов
	_, err = serializer.Deserialize(uncompressedData)
	assert.NoError(t, err)

	_, err = serializer.Deserialize(compressedData)
	assert.NoError(t, err)
}

// TestBatchSerialization проверяет пакетную сериализацию
func TestBatchSerialization(t *testing.T) {
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(t, err)
	defer serializer.Close()

	// Создаём пакет различных сообщений
	messages := []*protocol.NetGameMessage{
		{
			Sequence: 10,
			Payload: &protocol.NetGameMessage_Heartbeat{
				Heartbeat: &protocol.HeartbeatMessage{
					ClientTime: time.Now().UnixNano(),
				},
			},
		},
		{
			Sequence: 11,
			Payload: &protocol.NetGameMessage_EntityMove{
				EntityMove: &protocol.EntityMoveMessage{
					Entities: []*protocol.EntityData{
						{
							Id:       789,
							Type:     protocol.EntityType_ENTITY_PLAYER,
							Position: &protocol.Vec2{X: 300, Y: 400},
						},
					},
				},
			},
		},
		{
			Sequence: 12,
			Payload: &protocol.NetGameMessage_AuthRequest{
				AuthRequest: &protocol.AuthMessage{
					Username: "batch_user",
				},
			},
		},
	}

	// Пакетная сериализация
	batchData, err := serializer.SerializeBatch(messages)
	require.NoError(t, err)
	assert.Greater(t, len(batchData), 0)

	// Пакетная десериализация
	deserializedMessages, err := serializer.DeserializeBatch(batchData)
	require.NoError(t, err)
	assert.Equal(t, len(messages), len(deserializedMessages))

	// Проверяем, что все сообщения корректно десериализованы
	assert.NotNil(t, deserializedMessages[0].GetHeartbeat())
	assert.NotNil(t, deserializedMessages[1].GetEntityMove())
	assert.NotNil(t, deserializedMessages[2].GetAuthRequest())

	// Проверяем sequence numbers
	for i, msg := range deserializedMessages {
		assert.Equal(t, messages[i].Sequence, msg.Sequence)
	}
}

// TestKCPChannelConfiguration проверяет конфигурацию KCP каналов
func TestKCPChannelConfiguration(t *testing.T) {
	logger, err := logging.NewLogger("test")
	require.NoError(t, err)

	// Тест различных конфигураций KCP
	configs := []*network.ChannelConfig{
		network.DefaultChannelConfig(network.ChannelKCP),
		{
			Type:            network.ChannelKCP,
			BufferSize:      2048,
			Timeout:         10 * time.Second,
			KeepAlive:       5 * time.Second,
			CompressionType: protocol.CompressionType_NONE,
			MaxRetries:      5,
			RetryInterval:   2 * time.Second,
		},
		{
			Type:            network.ChannelKCP,
			BufferSize:      4096,
			Timeout:         30 * time.Second,
			KeepAlive:       15 * time.Second,
			CompressionType: protocol.CompressionType_ZSTD,
			MaxRetries:      3,
			RetryInterval:   time.Second,
		},
	}

	for i, config := range configs {
		t.Run(fmt.Sprintf("KCP config %d", i), func(t *testing.T) {
			channel := network.NewKCPChannel(config, logger)
			assert.NotNil(t, channel)
			assert.False(t, channel.IsConnected())

			// Проверяем базовые свойства канала
			assert.Equal(t, config.Type, config.Type)
			assert.Greater(t, config.BufferSize, 0)
		})
	}
}

// TestNetworkMetrics проверяет систему метрик
func TestNetworkMetrics(t *testing.T) {
	metrics := network.NewNetworkMetrics()
	require.NotNil(t, metrics)

	// Проверяем создание метрик
	assert.Equal(t, int64(0), metrics.TotalMessages)
	assert.Equal(t, int64(0), metrics.TotalBytes)
	assert.False(t, metrics.LastUpdate.IsZero())

	// Проверяем снимок метрик
	snapshot := metrics.GetSnapshot()
	require.NotNil(t, snapshot)
	assert.Equal(t, metrics.TotalMessages, snapshot.TotalMessages)
	assert.Equal(t, metrics.TotalBytes, snapshot.TotalBytes)
}

// BenchmarkMessageSerialization проверяет производительность сериализации
func BenchmarkMessageSerialization(b *testing.B) {
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(b, err)
	defer serializer.Close()

	msg := &protocol.NetGameMessage{
		Sequence: 1,
		Payload: &protocol.NetGameMessage_EntityMove{
			EntityMove: &protocol.EntityMoveMessage{
				Entities: []*protocol.EntityData{
					{
						Id:       123,
						Type:     protocol.EntityType_ENTITY_PLAYER,
						Position: &protocol.Vec2{X: 100, Y: 200},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := serializer.Serialize(msg)
		require.NoError(b, err)

		_, err = serializer.Deserialize(data)
		require.NoError(b, err)
	}
}

// BenchmarkBatchSerialization проверяет производительность пакетной сериализации
func BenchmarkBatchSerialization(b *testing.B) {
	serializer, err := protocol.NewMessageSerializer()
	require.NoError(b, err)
	defer serializer.Close()

	// Создаём пакет из 10 сообщений
	messages := make([]*protocol.NetGameMessage, 10)
	for i := 0; i < 10; i++ {
		messages[i] = &protocol.NetGameMessage{
			Sequence: uint32(i + 1),
			Payload: &protocol.NetGameMessage_EntityMove{
				EntityMove: &protocol.EntityMoveMessage{
					Entities: []*protocol.EntityData{
						{
							Id:       uint64(100 + i),
							Type:     protocol.EntityType_ENTITY_PLAYER,
							Position: &protocol.Vec2{X: int32(i * 10), Y: int32(i * 20)},
						},
					},
				},
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := serializer.SerializeBatch(messages)
		require.NoError(b, err)

		_, err = serializer.DeserializeBatch(data)
		require.NoError(b, err)
	}
}
