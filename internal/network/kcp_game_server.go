package network

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/storage"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// KCPGameServer представляет игровой сервер с поддержкой KCP протокола
type KCPGameServer struct {
	kcpServer    *ChannelServer
	udpServer    *UDPServerPB // Оставляем UDP для fallback
	worldManager *world.WorldManager
	gameHandler  *GameHandlerPB
	gameAuth     *auth.GameAuthenticator
	logger       *logging.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewKCPGameServer создает новый игровой сервер с поддержкой KCP
func NewKCPGameServer(kcpAddr, udpAddr string) (*KCPGameServer, error) {
	// Создаем контекст с отменой
	ctx, cancel := context.WithCancel(context.Background())

	// Инициализируем логгер
	logger := logging.GetNetworkLogger()

	// Инициализируем менеджер мира с идентификатором мира
	worldManager := world.NewWorldManager(1234)

	// Создаем менеджер сущностей
	entityManager := entity.NewEntityManager()

	// Подготавливаем репозиторий пользователей (in-memory)
	userRepo, err := auth.NewMemoryUserRepo()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create user repository: %w", err)
	}

	// Генерируем 32-байтовый секрет и регистрируем его глобально в пакете auth
	jwtSecret := make([]byte, 32)
	if _, err := rand.Read(jwtSecret); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	// Устанавливаем секрет (base64) в пакет auth
	if err := auth.SetJWTSecret(base64.StdEncoding.EncodeToString(jwtSecret)); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to set JWT secret: %w", err)
	}

	// Создаем аутентификатор
	gameAuth := auth.NewGameAuthenticator(userRepo, jwtSecret)
	logger.Info("🔐 KCP GameAuthenticator инициализирован с JWT поддержкой")

	// Создаем обработчик игровых сообщений
	gameHandler := NewGameHandlerPB(worldManager, entityManager, userRepo)

	// Создаем KCP сервер
	kcpConfig := DefaultChannelConfig(ChannelKCP)
	kcpServer := NewChannelServer(kcpAddr, kcpConfig)

	// Создаем UDP-сервер для fallback
	udpServer, err := NewUDPServerPB(udpAddr, worldManager)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create UDP server: %w", err)
	}

	// Настраиваем обработчики KCP сервера
	kcpServer.SetHandlers(
		func(clientID string, channel NetChannel) {
			// Обработчик подключения
			logger.Info("🔗 KCP клиент подключен: %s", clientID)

			// TODO: Интегрировать с GameHandler для аутентификации
		},
		func(clientID string) {
			// Обработчик отключения
			logger.Info("👋 KCP клиент отключен: %s", clientID)

			// TODO: Уведомить GameHandler об отключении
		},
		func(clientID string, msg *protocol.GameMessage) {
			// Обработчик сообщений
			logger.Debug("📨 Получено KCP сообщение от %s: %v", clientID, msg.Type)

			// TODO: Передать сообщение в GameHandler
		},
	)

	// Связываем компоненты
	gameHandler.SetGameAuthenticator(gameAuth)
	udpServer.SetGameHandler(gameHandler)

	return &KCPGameServer{
		kcpServer:    kcpServer,
		udpServer:    udpServer,
		worldManager: worldManager,
		gameHandler:  gameHandler,
		gameAuth:     gameAuth,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start запускает KCP игровой сервер
func (kgs *KCPGameServer) Start() error {
	// Запускаем KCP сервер
	if err := kgs.kcpServer.Start(); err != nil {
		return fmt.Errorf("failed to start KCP server: %w", err)
	}

	// Запускаем UDP сервер для fallback
	kgs.udpServer.Start()

	// Запускаем обработку мира
	kgs.worldManager.Run(kgs.ctx)

	// Создаем горутину для обновления игры
	kgs.wg.Add(1)
	go func() {
		defer kgs.wg.Done()

		// Тикер для обновления игры с частотой 20 тиков в секунду
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		var lastTick time.Time = time.Now()

		for {
			select {
			case <-kgs.ctx.Done():
				return
			case tickTime := <-ticker.C:
				// Вычисляем дельту времени между тиками
				dt := tickTime.Sub(lastTick).Seconds()
				lastTick = tickTime

				// Обновляем обработчик игры
				kgs.gameHandler.Tick(dt)
			}
		}
	}()

	kgs.logger.Info("🎮 KCP игровой сервер запущен (KCP: %s, UDP fallback: %s)",
		"kcp://"+kgs.kcpServer.addr, kgs.udpServer.conn.LocalAddr())
	return nil
}

// Stop останавливает KCP игровой сервер
func (kgs *KCPGameServer) Stop() {
	kgs.logger.Info("🛑 Остановка KCP игрового сервера...")

	// Отменяем контекст
	kgs.cancel()

	// Останавливаем KCP сервер
	if err := kgs.kcpServer.Stop(); err != nil {
		kgs.logger.Error("❌ Ошибка остановки KCP сервера: %v", err)
	}

	// Останавливаем UDP сервер
	kgs.udpServer.Stop()

	// Ждем завершения всех горутин
	kgs.wg.Wait()

	kgs.logger.Info("✅ KCP игровой сервер остановлен")
}

// SetPositionRepo устанавливает репозиторий позиций игроков
func (kgs *KCPGameServer) SetPositionRepo(positionRepo storage.PositionRepo) {
	if kgs.gameHandler != nil {
		kgs.gameHandler.SetPositionRepo(positionRepo)
	}
}

// GetConnectedClients возвращает количество подключенных клиентов
func (kgs *KCPGameServer) GetConnectedClients() int {
	if kgs.kcpServer != nil {
		return kgs.kcpServer.GetClientCount()
	}
	return 0
}
