package network

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"log"
	"net"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
)

// UDPServer обрабатывает UDP соединения для некритичных данных
type UDPServer struct {
	conn            *net.UDPConn
	clientAddresses map[uint64]*net.UDPAddr // Адреса клиентов (PlayerID -> UDPAddr)
	worldManager    *world.WorldManager
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	// Добавляем кэш последних известных позиций сущностей
	entityPositions map[uint64]vec.Vec2  // Кэш позиций (EntityID -> Position)
	entityCache     sync.RWMutex         // Мьютекс для кэша сущностей
	lastPingTime    map[uint64]time.Time // Время последнего пинга от клиента
}

// NewUDPServer создаёт новый UDP сервер
func NewUDPServer(address string, worldManager *world.WorldManager) (*UDPServer, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &UDPServer{
		conn:            conn,
		clientAddresses: make(map[uint64]*net.UDPAddr),
		worldManager:    worldManager,
		ctx:             ctx,
		cancel:          cancel,
		entityPositions: make(map[uint64]vec.Vec2),
		lastPingTime:    make(map[uint64]time.Time),
	}, nil
}

// Start запускает UDP сервер
func (s *UDPServer) Start() {
	go s.receiveLoop()
	go s.entitiesUpdateLoop()
}

// Stop останавливает UDP сервер
func (s *UDPServer) Stop() {
	s.cancel()
	s.conn.Close()
}

// RegisterClient регистрирует нового клиента
func (s *UDPServer) RegisterClient(playerID uint64, addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clientAddresses[playerID] = addr
	log.Printf("UDP: Зарегистрирован клиент %d с адресом %s", playerID, addr.String())
}

// UnregisterClient удаляет клиента
func (s *UDPServer) UnregisterClient(playerID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clientAddresses, playerID)
	log.Printf("UDP: Удалён клиент %d", playerID)

	// Также удаляем данные о позиции, если это игрок
	s.entityCache.Lock()
	delete(s.entityPositions, playerID)
	s.entityCache.Unlock()
}

// receiveLoop принимает UDP пакеты
func (s *UDPServer) receiveLoop() {
	buffer := make([]byte, 1024)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			// Устанавливаем таймаут чтения, чтобы можно было проверять контекст
			s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

			n, addr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Это таймаут, просто продолжаем
					continue
				}
				log.Printf("Ошибка чтения UDP: %v", err)
				continue
			}

			// Обрабатываем полученный пакет
			s.handlePacket(buffer[:n], addr)
		}
	}
}

// entitiesUpdateLoop периодически отправляет обновления о позициях сущностей
func (s *UDPServer) entitiesUpdateLoop() {
	ticker := time.NewTicker(100 * time.Millisecond) // 10 раз в секунду
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Отправляем обновления позиций сущностей всем клиентам
			s.broadcastEntityPositions()
		}
	}
}

// broadcastEntityPositions отправляет всем клиентам обновления позиций сущностей
func (s *UDPServer) broadcastEntityPositions() {
	// Копируем кэш позиций, чтобы не держать блокировку при отправке
	s.entityCache.RLock()
	entities := make([]world.EntityData, 0, len(s.entityPositions))
	for id, pos := range s.entityPositions {
		entities = append(entities, world.EntityData{
			ID:       id,
			Position: pos,
		})
	}
	s.entityCache.RUnlock()

	// Если нет данных для отправки, выходим
	if len(entities) == 0 {
		return
	}

	// Отправляем обновления всем клиентам
	s.mu.RLock()
	defer s.mu.RUnlock()

	for playerID, addr := range s.clientAddresses {
		// Фильтруем сущности - не отправляем клиенту его собственную позицию
		filteredEntities := make([]world.EntityData, 0, len(entities))
		for _, entity := range entities {
			if entity.ID != playerID {
				filteredEntities = append(filteredEntities, entity)
			}
		}

		// Если после фильтрации нет данных, пропускаем клиента
		if len(filteredEntities) == 0 {
			continue
		}

		// Формируем пакет с обновлениями
		header := make([]byte, 6)
		binary.BigEndian.PutUint16(header[0:2], uint16(MsgEntityMove))
		binary.BigEndian.PutUint32(header[2:6], uint32(len(filteredEntities)))

		// Данные сущностей
		entityData := make([]byte, len(filteredEntities)*16)
		for i, entity := range filteredEntities {
			offset := i * 16
			binary.BigEndian.PutUint64(entityData[offset:offset+8], entity.ID)
			binary.BigEndian.PutUint32(entityData[offset+8:offset+12], uint32(entity.Position.X))
			binary.BigEndian.PutUint32(entityData[offset+12:offset+16], uint32(entity.Position.Y))
		}

		// Объединяем данные
		packet := append(header, entityData...)

		// Отправляем пакет
		_, err := s.conn.WriteToUDP(packet, addr)
		if err != nil {
			log.Printf("Ошибка отправки UDP: %v", err)
		}
	}
}

// handlePacket обрабатывает полученный UDP пакет
func (s *UDPServer) handlePacket(data []byte, addr *net.UDPAddr) {
	if len(data) < 6 {
		// Слишком короткий пакет
		return
	}

	// Формат пакета:
	// [0-3]: PlayerID (uint32)
	// [4-5]: MessageType (uint16)
	// [6+]: Payload

	playerID := binary.BigEndian.Uint32(data[0:4])
	msgType := binary.BigEndian.Uint16(data[4:6])
	payload := data[6:]

	switch msgType {
	case 0: // MsgEntityMove
		s.handleEntityMove(uint64(playerID), payload, addr)
	case 2: // MsgPing (значение 2, отправляемое клиентом)
		s.handlePing(uint64(playerID), payload, addr)
	default:
		log.Printf("UDP: Неизвестный тип сообщения: %d от %s", msgType, addr.String())
		log.Printf("UDP: Получен пакет от %s, PlayerID: %d, MessageType: %d", addr.String(), playerID, msgType)
		log.Printf("UDP: Пакет в виде HEX: %s", hex.EncodeToString(data))
	}
}

// handleEntityMove обрабатывает сообщение о перемещении сущности
func (s *UDPServer) handleEntityMove(playerID uint64, data []byte, addr *net.UDPAddr) {
	// Регистрируем адрес клиента, если не зарегистрирован
	s.mu.RLock()
	_, exists := s.clientAddresses[playerID]
	s.mu.RUnlock()

	if !exists {
		s.RegisterClient(playerID, addr)
	}

	// Формат данных о перемещении:
	// [0-3]: X (int32)
	// [4-7]: Y (int32)
	// [8-11]: Флаги (uint32) - для будущего использования (направление взгляда, анимация и т.д.)
	if len(data) < 8 {
		return
	}

	// Получаем координаты
	x := int(binary.BigEndian.Uint32(data[0:4]))
	y := int(binary.BigEndian.Uint32(data[4:8]))

	// Флаги (если есть)
	var flags uint32
	if len(data) >= 12 {
		flags = binary.BigEndian.Uint32(data[8:12])
	}

	// Создаем новую позицию
	newPos := vec.Vec2{X: x, Y: y}

	// Получаем предыдущую позицию из кэша
	var oldPos vec.Vec2
	s.entityCache.RLock()
	oldPos, exists = s.entityPositions[playerID]
	s.entityCache.RUnlock()

	// Обновляем позицию в кэше
	s.entityCache.Lock()
	s.entityPositions[playerID] = newPos
	s.entityCache.Unlock()

	// Если это первое сообщение или позиция изменилась значительно,
	// обрабатываем перемещение через WorldManager
	if !exists || oldPos.DistanceTo(newPos) >= 16 {
		// Значительное перемещение, обрабатываем через WorldManager
		s.worldManager.ProcessEntityMovement(playerID, oldPos, newPos)
		log.Printf("UDP: Игрок %d переместился значительно из (%d, %d) в (%d, %d)",
			playerID, oldPos.X, oldPos.Y, x, y)
	} else {
		// Мелкое перемещение, просто обновляем кэш
		log.Printf("UDP: Игрок %d переместился в (%d, %d), флаги: %d", playerID, x, y, flags)
	}

	// Проверяем, не требуется ли обработка специальных флагов
	if flags != 0 {
		// Например, флаг начала бега, прыжка и т.д.
		s.processMovementFlags(playerID, newPos, flags)
	}
}

// processMovementFlags обрабатывает специальные флаги движения
func (s *UDPServer) processMovementFlags(playerID uint64, pos vec.Vec2, flags uint32) {
	// Это заглушка для будущей реализации
	// Здесь может быть обработка специальных типов движения, анимаций и т.д.

	// Примеры флагов:
	// 0x01 - бег
	// 0x02 - прыжок
	// 0x04 - приседание
	// 0x08 - начало атаки

	if (flags & 0x01) != 0 {
		log.Printf("UDP: Игрок %d начал бежать", playerID)
	}

	if (flags & 0x02) != 0 {
		log.Printf("UDP: Игрок %d прыгнул", playerID)
	}
}

// SendEntityUpdates отправляет обновления о сущностях клиенту
func (s *UDPServer) SendEntityUpdates(playerID uint64, entities []world.EntityData) {
	s.mu.RLock()
	addr, exists := s.clientAddresses[playerID]
	s.mu.RUnlock()

	if !exists {
		return
	}

	// Формируем пакет с обновлениями
	// Формат:
	// [0-1]: MessageType (uint16 = MsgEntityMove)
	// [2-5]: Count (uint32)
	// Для каждой сущности:
	//   [0-7]: EntityID (uint64)
	//   [8-11]: X (int32)
	//   [12-15]: Y (int32)

	// Заголовок
	header := make([]byte, 6)
	binary.BigEndian.PutUint16(header[0:2], uint16(MsgEntityMove))
	binary.BigEndian.PutUint32(header[2:6], uint32(len(entities)))

	// Данные сущностей
	entityData := make([]byte, len(entities)*16)
	for i, entity := range entities {
		offset := i * 16
		binary.BigEndian.PutUint64(entityData[offset:offset+8], entity.ID)
		binary.BigEndian.PutUint32(entityData[offset+8:offset+12], uint32(entity.Position.X))
		binary.BigEndian.PutUint32(entityData[offset+12:offset+16], uint32(entity.Position.Y))

		// Обновляем кэш позиций
		s.entityCache.Lock()
		s.entityPositions[entity.ID] = entity.Position
		s.entityCache.Unlock()
	}

	// Объединяем данные
	packet := append(header, entityData...)

	// Отправляем пакет
	_, err := s.conn.WriteToUDP(packet, addr)
	if err != nil {
		log.Printf("Ошибка отправки UDP: %v", err)
	}
}

// UpdateEntityPosition обновляет позицию сущности в кэше
func (s *UDPServer) UpdateEntityPosition(entityID uint64, position vec.Vec2) {
	s.entityCache.Lock()
	s.entityPositions[entityID] = position
	s.entityCache.Unlock()
}

// GetEntityPosition возвращает последнюю известную позицию сущности
func (s *UDPServer) GetEntityPosition(entityID uint64) (vec.Vec2, bool) {
	s.entityCache.RLock()
	pos, exists := s.entityPositions[entityID]
	s.entityCache.RUnlock()
	return pos, exists
}

// handlePing обрабатывает пинг-сообщения от клиентов
func (s *UDPServer) handlePing(playerID uint64, data []byte, addr *net.UDPAddr) {
	// Регистрируем адрес клиента, если не зарегистрирован
	s.mu.RLock()
	_, exists := s.clientAddresses[playerID]
	s.mu.RUnlock()

	if !exists {
		s.RegisterClient(playerID, addr)
	}

	// Обновляем время последнего пинга
	s.mu.Lock()
	s.lastPingTime[playerID] = time.Now()
	s.mu.Unlock()

	// Отправляем ответный пинг, если нужно
	// В данном случае просто логируем успешный пинг
	if len(data) > 0 {
		log.Printf("UDP: Получен пинг от игрока %d: %s", playerID, string(data))
	}
}
