package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
)

func main() {
	fmt.Println("=== ТЕСТОВЫЙ КЛИЕНТ ДЛЯ АНАЛИЗА ПРОТОКОЛА ===")

	// Подключаемся к серверу
	conn, err := net.Dial("tcp", "localhost:7777")
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer conn.Close()

	fmt.Println("✅ Подключен к серверу")

	serializer := protocol.NewMessageSerializer()

	// Тест 1: Аутентификация
	fmt.Println("\n=== ТЕСТ 1: АУТЕНТИФИКАЦИЯ ===")
	testAuth(conn, serializer)

	// Тест 2: Запрос чанка
	fmt.Println("\n=== ТЕСТ 2: ЗАПРОС ЧАНКА ===")
	testChunkRequest(conn, serializer)

	// Тест 3: UDP сообщения
	fmt.Println("\n=== ТЕСТ 3: UDP СООБЩЕНИЯ ===")
	testUDP()

	fmt.Println("\n=== ТЕСТИРОВАНИЕ ЗАВЕРШЕНО ===")
	time.Sleep(2 * time.Second)
}

func testAuth(conn net.Conn, serializer *protocol.MessageSerializer) {
	// Создаем запрос аутентификации
	password := "ChangeMe123!"
	authReq := &protocol.AuthRequest{
		Username: "admin",
		Password: &password,
	}

	// Сериализуем
	data, err := serializer.SerializeMessage(protocol.MessageType_AUTH, authReq)
	if err != nil {
		log.Printf("❌ Ошибка сериализации AUTH: %v", err)
		return
	}

	fmt.Printf("📤 Отправка AUTH сообщения (%d байт)\n", len(data))
	logHexDump("AUTH REQUEST", data)

	// Отправляем длину + данные
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))

	conn.Write(header)
	conn.Write(data)

	// Читаем ответ
	responseHeader := make([]byte, 4)
	_, err = conn.Read(responseHeader)
	if err != nil {
		log.Printf("❌ Ошибка чтения заголовка ответа: %v", err)
		return
	}

	responseSize := binary.BigEndian.Uint32(responseHeader)
	responseData := make([]byte, responseSize)
	_, err = conn.Read(responseData)
	if err != nil {
		log.Printf("❌ Ошибка чтения ответа: %v", err)
		return
	}

	fmt.Printf("📥 Получен ответ AUTH (%d байт)\n", len(responseData))
	logHexDump("AUTH RESPONSE", responseData)

	// Десериализуем ответ
	msg, err := serializer.DeserializeMessage(responseData)
	if err != nil {
		log.Printf("❌ Ошибка десериализации ответа: %v", err)
		return
	}

	authResp := &protocol.AuthResponse{}
	if err := serializer.DeserializePayload(msg, authResp); err != nil {
		log.Printf("❌ Ошибка десериализации AuthResponse: %v", err)
		return
	}

	if authResp.Success {
		fmt.Printf("✅ Аутентификация успешна! PlayerID: %d\n", authResp.PlayerId)
	} else {
		fmt.Printf("❌ Аутентификация неудачна: %s\n", authResp.Message)
	}
}

func testChunkRequest(conn net.Conn, serializer *protocol.MessageSerializer) {
	// Создаем запрос чанка
	chunkReq := &protocol.ChunkRequest{
		ChunkX: 0,
		ChunkY: 0,
	}

	// Сериализуем
	data, err := serializer.SerializeMessage(protocol.MessageType_CHUNK_REQUEST, chunkReq)
	if err != nil {
		log.Printf("❌ Ошибка сериализации CHUNK_REQUEST: %v", err)
		return
	}

	fmt.Printf("📤 Отправка CHUNK_REQUEST (%d байт)\n", len(data))
	logHexDump("CHUNK_REQUEST", data)

	// Отправляем длину + данные
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))

	conn.Write(header)
	conn.Write(data)

	// Читаем ответ
	responseHeader := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(responseHeader)
	if err != nil {
		log.Printf("❌ Ошибка чтения заголовка ответа чанка: %v", err)
		return
	}

	responseSize := binary.BigEndian.Uint32(responseHeader)
	responseData := make([]byte, responseSize)
	_, err = conn.Read(responseData)
	if err != nil {
		log.Printf("❌ Ошибка чтения ответа чанка: %v", err)
		return
	}

	fmt.Printf("📥 Получен ответ CHUNK_DATA (%d байт)\n", len(responseData))
	logHexDump("CHUNK_DATA", responseData[:min(len(responseData), 200)]) // Первые 200 байт

	// Десериализуем ответ
	msg, err := serializer.DeserializeMessage(responseData)
	if err != nil {
		log.Printf("❌ Ошибка десериализации ответа чанка: %v", err)
		return
	}

	chunkData := &protocol.ChunkData{}
	if err := serializer.DeserializePayload(msg, chunkData); err != nil {
		log.Printf("❌ Ошибка десериализации ChunkData: %v", err)
		return
	}

	fmt.Printf("✅ Получен чанк (%d,%d) с %d строками блоков\n",
		chunkData.ChunkX, chunkData.ChunkY, len(chunkData.Blocks))
}

func testUDP() {
	// Подключаемся по UDP
	conn, err := net.Dial("udp", "localhost:7778")
	if err != nil {
		log.Printf("❌ Ошибка UDP подключения: %v", err)
		return
	}
	defer conn.Close()

	fmt.Println("📡 Подключен к UDP серверу")

	serializer := protocol.NewMessageSerializer()

	// Создаем сообщение о движении
	moveMsg := &protocol.EntityMoveMessage{
		Entities: []*protocol.EntityData{
			{
				Id:       1,
				Position: &protocol.Vec2{X: 10, Y: 20},
				Velocity: &protocol.Vec2Float{X: 1.5, Y: 2.5},
				Type:     protocol.EntityType_ENTITY_PLAYER,
				Active:   true,
			},
		},
	}

	// Сериализуем
	data, err := serializer.SerializeMessage(protocol.MessageType_ENTITY_MOVE, moveMsg)
	if err != nil {
		log.Printf("❌ Ошибка сериализации ENTITY_MOVE: %v", err)
		return
	}

	// Создаем пакет с playerID в заголовке
	playerID := uint64(1)
	header := make([]byte, 8)
	binary.BigEndian.PutUint64(header, playerID)
	packet := append(header, data...)

	fmt.Printf("📤 Отправка UDP ENTITY_MOVE (%d байт)\n", len(packet))
	logHexDump("UDP ENTITY_MOVE", packet)

	// Отправляем
	_, err = conn.Write(packet)
	if err != nil {
		log.Printf("❌ Ошибка отправки UDP: %v", err)
		return
	}

	fmt.Println("✅ UDP сообщение отправлено")
}

func logHexDump(title string, data []byte) {
	fmt.Printf("=== %s HEX DUMP ===\n", title)
	const bytesPerLine = 16
	for i := 0; i < len(data); i += bytesPerLine {
		end := i + bytesPerLine
		if end > len(data) {
			end = len(data)
		}

		// Offset
		fmt.Printf("%08x: ", i)

		// Hex bytes
		for j := i; j < end; j++ {
			fmt.Printf("%02x ", data[j])
		}

		// Выравнивание
		for j := end; j < i+bytesPerLine; j++ {
			fmt.Printf("   ")
		}

		// ASCII
		fmt.Printf(" |")
		for j := i; j < end; j++ {
			if data[j] >= 32 && data[j] < 127 {
				fmt.Printf("%c", data[j])
			} else {
				fmt.Printf(".")
			}
		}
		fmt.Printf("|\n")
	}
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
