package network

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// GameServerPB представляет основной игровой сервер с поддержкой Protocol Buffers
type GameServerPB struct {
	tcpServer    *TCPServerPB
	udpServer    *UDPServerPB
	worldManager *world.WorldManager
	gameHandler  *GameHandlerPB
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewGameServerPB создает новый игровой сервер с поддержкой Protocol Buffers
func NewGameServerPB(tcpAddr, udpAddr string) (*GameServerPB, error) {
	// Создаем контекст с отменой
	ctx, cancel := context.WithCancel(context.Background())

	// Инициализируем менеджер мира с идентификатором мира
	worldManager := world.NewWorldManager(1234) // Используем числовой идентификатор мира

	// Создаем менеджер сущностей
	entityManager := entity.NewEntityManager()

	// Подготавливаем репозиторий пользователей (in-memory)
	userRepo, err := auth.NewMemoryUserRepo()
	if err != nil {
		cancel()
		return nil, err
	}

	// Создаем обработчик игровых сообщений
	gameHandler := NewGameHandlerPB(worldManager, entityManager, userRepo)

	// Создаем TCP-сервер
	tcpServer, err := NewTCPServerPB(tcpAddr, worldManager)
	if err != nil {
		cancel()
		return nil, err
	}

	// Создаем UDP-сервер
	udpServer, err := NewUDPServerPB(udpAddr, worldManager)
	if err != nil {
		cancel()
		return nil, err
	}

	// Связываем компоненты вместе
	tcpServer.SetGameHandler(gameHandler)
	gameHandler.SetTCPServer(tcpServer)
	gameHandler.SetUDPServer(udpServer)

	return &GameServerPB{
		tcpServer:    tcpServer,
		udpServer:    udpServer,
		worldManager: worldManager,
		gameHandler:  gameHandler,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start запускает игровой сервер
func (gs *GameServerPB) Start() error {
	// Запускаем TCP сервер
	gs.tcpServer.Start()

	// Запускаем UDP сервер
	gs.udpServer.Start()

	// Запускаем обработку мира
	gs.worldManager.Run(gs.ctx)

	// Создаем горутину для обработки игры
	gs.wg.Add(1)
	go func() {
		defer gs.wg.Done()

		// Тикер для обновления игры с частотой 20 тиков в секунду
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		var lastTick time.Time = time.Now()

		for {
			select {
			case <-gs.ctx.Done():
				return
			case tickTime := <-ticker.C:
				// Вычисляем дельту времени между тиками
				dt := tickTime.Sub(lastTick).Seconds()
				lastTick = tickTime

				// Обновляем обработчик игры
				gs.gameHandler.Tick(dt)
			}
		}
	}()

	log.Printf("Игровой сервер запущен (TCP: %s, UDP: %s)", gs.tcpServer.listener.Addr(), gs.udpServer.conn.LocalAddr())
	return nil
}

// Stop останавливает игровой сервер
func (s *GameServerPB) Stop() {
	// Отменяем контекст, чтобы остановить все горутины
	s.cancel()

	// Останавливаем TCP-сервер
	s.tcpServer.Stop()

	// Останавливаем UDP-сервер
	s.udpServer.Stop()

	// Ждем завершения всех горутин
	s.wg.Wait()

	log.Println("Игровой сервер остановлен")
}
