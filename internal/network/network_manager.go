package network

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
)

// NetworkManager управляет сетевым взаимодействием
type NetworkManager struct {
	tcpServer      *TCPServer
	udpServer      *UDPServer
	worldManager   *world.WorldManager
	tcpPort        int
	udpPort        int
	playerEntities map[uint64]uint64 // Связь клиентов и их сущностей (connectionID -> entityID)
	entityPlayers  map[uint64]uint64 // Обратная связь сущностей и клиентов (entityID -> connectionID)
	mu             sync.RWMutex
	visibilityMap  map[uint64]map[uint64]struct{} // Карта видимости (playerID -> set of entityIDs)
	visibilityMu   sync.RWMutex
}

// NewNetworkManager создаёт новый менеджер сети
func NewNetworkManager(tcpPort, udpPort int, worldManager *world.WorldManager) *NetworkManager {
	return &NetworkManager{
		worldManager:   worldManager,
		tcpPort:        tcpPort,
		udpPort:        udpPort,
		playerEntities: make(map[uint64]uint64),
		entityPlayers:  make(map[uint64]uint64),
		visibilityMap:  make(map[uint64]map[uint64]struct{}),
	}
}

// Start запускает сетевые серверы
func (nm *NetworkManager) Start() error {
	// Запускаем TCP сервер
	tcpServer, err := NewTCPServer(fmt.Sprintf(":%d", nm.tcpPort), nm.worldManager)
	if err != nil {
		return fmt.Errorf("ошибка запуска TCP сервера: %w", err)
	}
	nm.tcpServer = tcpServer
	nm.tcpServer.Start()
	log.Printf("TCP сервер запущен на порту %d", nm.tcpPort)

	// Запускаем UDP сервер
	udpServer, err := NewUDPServer(fmt.Sprintf(":%d", nm.udpPort), nm.worldManager)
	if err != nil {
		nm.tcpServer.Stop()
		return fmt.Errorf("ошибка запуска UDP сервера: %w", err)
	}
	nm.udpServer = udpServer
	nm.udpServer.Start()
	log.Printf("UDP сервер запущен на порту %d", nm.udpPort)

	// Запускаем фоновые задачи
	go nm.runEntityVisibilityLoop()

	return nil
}

// Stop останавливает сетевые серверы
func (nm *NetworkManager) Stop() {
	if nm.tcpServer != nil {
		nm.tcpServer.Stop()
		log.Println("TCP сервер остановлен")
	}

	if nm.udpServer != nil {
		nm.udpServer.Stop()
		log.Println("UDP сервер остановлен")
	}
}

// RegisterPlayerEntity регистрирует связь между клиентом и сущностью игрока
func (nm *NetworkManager) RegisterPlayerEntity(connectionID, entityID uint64) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nm.playerEntities[connectionID] = entityID
	nm.entityPlayers[entityID] = connectionID

	// Инициализируем карту видимости для игрока
	nm.visibilityMu.Lock()
	nm.visibilityMap[entityID] = make(map[uint64]struct{})
	nm.visibilityMu.Unlock()
}

// UnregisterPlayerEntity удаляет связь между клиентом и сущностью игрока
func (nm *NetworkManager) UnregisterPlayerEntity(connectionID uint64) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	entityID, exists := nm.playerEntities[connectionID]
	if exists {
		delete(nm.playerEntities, connectionID)
		delete(nm.entityPlayers, entityID)

		// Удаляем из карты видимости
		nm.visibilityMu.Lock()
		delete(nm.visibilityMap, entityID)
		nm.visibilityMu.Unlock()

		// Удаляем из UDP-сервера
		if nm.udpServer != nil {
			nm.udpServer.UnregisterClient(entityID)
		}
	}
}

// runEntityVisibilityLoop периодически обновляет карту видимости сущностей для каждого игрока
func (nm *NetworkManager) runEntityVisibilityLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			nm.updateEntityVisibility()
		}
	}
}

// updateEntityVisibility обновляет карту видимости для всех игроков
func (nm *NetworkManager) updateEntityVisibility() {
	// Получаем списки всех активных игроков
	nm.mu.RLock()
	playerIDs := make([]uint64, 0, len(nm.entityPlayers))
	for playerID := range nm.entityPlayers {
		playerIDs = append(playerIDs, playerID)
	}
	nm.mu.RUnlock()

	// Для каждого игрока определяем видимые сущности
	for _, playerID := range playerIDs {
		// Получаем текущую позицию игрока
		playerPos, ok := nm.udpServer.GetEntityPosition(playerID)
		if !ok {
			continue // Игрок не имеет позиции
		}

		// Создаем временное множество для новых видимых сущностей
		newVisible := make(map[uint64]struct{})

		// Получаем все сущности в радиусе видимости
		// TODO: Реализовать запрос к WorldManager для получения сущностей в радиусе
		// Пока используем все сущности из кэша UDP-сервера
		nm.mu.RLock()
		for entityID := range nm.entityPlayers {
			if entityID != playerID { // Исключаем самого игрока
				entityPos, ok := nm.udpServer.GetEntityPosition(entityID)
				if ok {
					// Проверяем расстояние (в блоках)
					distance := playerPos.DistanceTo(entityPos)
					if distance <= 100 { // Радиус видимости 100 блоков
						newVisible[entityID] = struct{}{}
					}
				}
			}
		}
		nm.mu.RUnlock()

		// Обновляем карту видимости для этого игрока
		nm.visibilityMu.Lock()
		nm.visibilityMap[playerID] = newVisible
		nm.visibilityMu.Unlock()
	}
}

// BroadcastEntityUpdates отправляет обновления о сущностях всем клиентам
func (nm *NetworkManager) BroadcastEntityUpdates(entities []world.EntityData) {
	// Отправляем только через UDP, т.к. это некритичные данные
	if nm.udpServer == nil {
		return
	}

	// Для каждого игрока отправляем только видимые для него сущности
	nm.mu.RLock()
	for playerID := range nm.entityPlayers {
		// Фильтруем сущности по видимости
		visibleEntities := nm.filterVisibleEntities(playerID, entities)

		// Если есть видимые сущности, отправляем их
		if len(visibleEntities) > 0 {
			nm.udpServer.SendEntityUpdates(playerID, visibleEntities)
		}
	}
	nm.mu.RUnlock()
}

// filterVisibleEntities фильтрует сущности по видимости для указанного игрока
func (nm *NetworkManager) filterVisibleEntities(playerID uint64, entities []world.EntityData) []world.EntityData {
	nm.visibilityMu.RLock()
	defer nm.visibilityMu.RUnlock()

	visibleMap, exists := nm.visibilityMap[playerID]
	if !exists {
		return nil
	}

	visibleEntities := make([]world.EntityData, 0, len(visibleMap))
	for _, entity := range entities {
		if _, visible := visibleMap[entity.ID]; visible || entity.ID == playerID {
			visibleEntities = append(visibleEntities, entity)
		}
	}

	return visibleEntities
}

// SendChunkToPlayer отправляет данные чанка игроку
func (nm *NetworkManager) SendChunkToPlayer(playerID uint64, chunk *world.Chunk) {
	if nm.tcpServer == nil {
		return
	}

	// Конвертируем чанк в формат для отправки
	chunkData := ConvertChunkToJSON(chunk)

	// Сериализуем данные
	data, err := json.Marshal(chunkData)
	if err != nil {
		log.Printf("Ошибка сериализации чанка: %v", err)
		return
	}

	// Находим соединение игрока
	nm.mu.RLock()
	connectionID, exists := nm.entityPlayers[playerID]
	nm.mu.RUnlock()

	if !exists {
		log.Printf("Не найдено соединение для игрока %d", playerID)
		return
	}

	// Отправляем данные через TCP (используем константу 1 - MsgChunkData)
	nm.tcpServer.mu.RLock()
	conn, exists := nm.tcpServer.connections[connectionID]
	nm.tcpServer.mu.RUnlock()

	if exists {
		conn.sendMessage(MessageType(1), data)
	}
}

// SendBlockUpdate отправляет обновление блока всем клиентам в зоне видимости
func (nm *NetworkManager) SendBlockUpdate(blockPos vec.Vec2, block world.Block) {
	if nm.tcpServer == nil {
		return
	}

	// Создаем данные об обновлении блока
	blockUpdate := JSONBlockUpdateRequest{
		X:       blockPos.X,
		Y:       blockPos.Y,
		BlockID: block.ID,
		Payload: block.Payload,
	}

	// Сериализуем данные
	data, err := json.Marshal(blockUpdate)
	if err != nil {
		log.Printf("Ошибка сериализации обновления блока: %v", err)
		return
	}

	// Отправляем всем клиентам (используем константу 2 - MsgBlockUpdate)
	nm.tcpServer.broadcastMessage(MessageType(2), data)

	log.Printf("Отправлено обновление блока на позиции (%d, %d) всем клиентам", blockPos.X, blockPos.Y)
}

// SendEntitySpawn отправляет информацию о создании сущности всем клиентам в зоне видимости
func (nm *NetworkManager) SendEntitySpawn(entityID uint64, entityType uint16, position vec.Vec2, metadata map[string]interface{}) {
	if nm.tcpServer == nil {
		return
	}

	// Создаем данные о сущности
	entityData := JSONEntityData{
		ID:       entityID,
		Type:     entityType,
		Position: position,
		Metadata: metadata,
	}

	// Создаем сообщение о создании сущности
	spawnData := map[string]interface{}{
		"entity": entityData,
	}

	// Сериализуем данные
	data, err := json.Marshal(spawnData)
	if err != nil {
		log.Printf("Ошибка сериализации данных о создании сущности: %v", err)
		return
	}

	// Отправляем всем клиентам (используем константу 3 - MsgEntitySpawn)
	nm.tcpServer.broadcastMessage(MessageType(3), data)

	// Также обновляем позицию в UDP-сервере
	if nm.udpServer != nil {
		nm.udpServer.UpdateEntityPosition(entityID, position)
	}

	log.Printf("Отправлена информация о создании сущности %d типа %d на позиции (%d, %d)",
		entityID, entityType, position.X, position.Y)
}

// SendEntityDespawn отправляет информацию об удалении сущности всем клиентам в зоне видимости
func (nm *NetworkManager) SendEntityDespawn(entityID uint64) {
	if nm.tcpServer == nil {
		return
	}

	// Создаем сообщение об удалении сущности
	despawnData := map[string]interface{}{
		"entity_id": entityID,
	}

	// Сериализуем данные
	data, err := json.Marshal(despawnData)
	if err != nil {
		log.Printf("Ошибка сериализации данных об удалении сущности: %v", err)
		return
	}

	// Отправляем всем клиентам (в будущем можно фильтровать по видимости)
	// Используем константу 7 (MsgEntityDespawn)
	nm.tcpServer.broadcastMessage(MessageType(7), data)

	// Также удаляем позицию из UDP-сервера
	if nm.udpServer != nil {
		nm.udpServer.entityCache.Lock()
		delete(nm.udpServer.entityPositions, entityID)
		nm.udpServer.entityCache.Unlock()
	}

	log.Printf("Отправлена информация об удалении сущности %d", entityID)
}

// SendChatMessage отправляет сообщение чата клиентам
func (nm *NetworkManager) SendChatMessage(senderID uint64, senderName, message string, chatType JSONChatMessageType) {
	if nm.tcpServer == nil {
		return
	}

	// Создаем сообщение чата
	chatMsg := JSONChatResponse{
		Type:       chatType,
		Message:    message,
		SenderID:   senderID,
		SenderName: senderName,
		Timestamp:  time.Now().Unix(),
	}

	// Сериализуем данные
	data, err := json.Marshal(chatMsg)
	if err != nil {
		log.Printf("Ошибка сериализации сообщения чата: %v", err)
		return
	}

	// В зависимости от типа чата, отправляем разным получателям
	switch chatType {
	case JSONChatGlobal:
		// Глобальный чат - всем (используем константу 6 - MsgChat)
		nm.tcpServer.broadcastMessage(MessageType(6), data)

	case JSONChatLocal:
		// Локальный чат - только тем, кто в радиусе видимости
		nm.visibilityMu.RLock()
		visiblePlayers, exists := nm.visibilityMap[senderID]
		nm.visibilityMu.RUnlock()

		if exists {
			nm.mu.RLock()
			for playerID := range visiblePlayers {
				connID, ok := nm.entityPlayers[playerID]
				if ok {
					nm.tcpServer.mu.RLock()
					conn, ok := nm.tcpServer.connections[connID]
					nm.tcpServer.mu.RUnlock()

					if ok {
						conn.sendMessage(MessageType(6), data)
					}
				}
			}

			// Отправляем также самому отправителю
			connID, ok := nm.entityPlayers[senderID]
			if ok {
				nm.tcpServer.mu.RLock()
				conn, ok := nm.tcpServer.connections[connID]
				nm.tcpServer.mu.RUnlock()

				if ok {
					conn.sendMessage(MessageType(6), data)
				}
			}
			nm.mu.RUnlock()
		}

	case JSONChatPrivate:
		// Private chat is handled separately via SendPrivateMessage
		log.Printf("Для приватных сообщений используйте SendPrivateMessage")
	}
}

// SendPrivateMessage отправляет приватное сообщение между двумя игроками
func (nm *NetworkManager) SendPrivateMessage(senderID, targetID uint64, senderName, message string) bool {
	if nm.tcpServer == nil {
		return false
	}

	// Создаем сообщение чата
	chatMsg := JSONChatResponse{
		Type:       JSONChatPrivate,
		Message:    message,
		SenderID:   senderID,
		SenderName: senderName,
		Timestamp:  time.Now().Unix(),
	}

	// Сериализуем данные
	data, err := json.Marshal(chatMsg)
	if err != nil {
		log.Printf("Ошибка сериализации приватного сообщения: %v", err)
		return false
	}

	var success bool

	// Отправляем получателю
	nm.mu.RLock()
	targetConnID, ok := nm.entityPlayers[targetID]
	if ok {
		nm.tcpServer.mu.RLock()
		targetConn, ok := nm.tcpServer.connections[targetConnID]
		nm.tcpServer.mu.RUnlock()

		if ok {
			targetConn.sendMessage(MessageType(6), data)
			success = true
		}
	}

	// Отправляем копию отправителю
	senderConnID, ok := nm.entityPlayers[senderID]
	if ok {
		nm.tcpServer.mu.RLock()
		senderConn, ok := nm.tcpServer.connections[senderConnID]
		nm.tcpServer.mu.RUnlock()

		if ok {
			senderConn.sendMessage(MessageType(6), data)
		}
	}
	nm.mu.RUnlock()

	return success
}

// UpdateEntityPosition обновляет позицию сущности в кэше
func (nm *NetworkManager) UpdateEntityPosition(entityID uint64, position vec.Vec2) {
	if nm.udpServer != nil {
		nm.udpServer.UpdateEntityPosition(entityID, position)
	}
}
