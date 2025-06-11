package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/network"
)

func main() {
	// Инициализируем систему логирования (используем новый API)
	if err := logging.InitDefaultLogger("server"); err != nil {
		log.Fatalf("❌ Ошибка инициализации логирования: %v", err)
	}
	defer logging.CloseDefaultLogger()

	logging.Info("🎮 Запуск игрового сервера с поддержкой JWT аутентификации...")
	logging.Debug("Инициализация системы логирования завершена")

	// === ОБНОВЛЕННЫЕ АДРЕСА ===
	// Новые порты для совместимости с обновленным клиентом
	tcpAddr := ":7777"
	udpAddr := ":7778"

	logging.Info("📡 Конфигурация сервера: TCP=%s, UDP=%s", tcpAddr, udpAddr)

	// Создаем игровой сервер
	logging.Debug("Создание игрового сервера...")
	server, err := network.NewGameServerPB(tcpAddr, udpAddr)
	if err != nil {
		logging.Error("❌ Ошибка создания сервера: %v", err)
		log.Fatalf("❌ Ошибка создания сервера: %v", err)
	}

	// Запускаем сервер
	logging.Debug("Запуск сервера...")
	server.Start()
	logging.Info("✅ Сервер запущен и готов принимать соединения")
	logging.Info("   🎮 Игровой трафик: TCP %s, UDP %s", tcpAddr, udpAddr)
	logging.Info("   🔐 JWT аутентификация активирована")
	logging.Debug("Сервер полностью инициализирован и работает")

	// Канал для получения сигналов ОС
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logging.Debug("Ожидание сигналов завершения...")

	// Ждем сигнала для завершения
	sig := <-sigCh
	logging.Info("📡 Получен сигнал %v, завершение работы...", sig)

	// Останавливаем сервер
	logging.Debug("Остановка сервера...")
	server.Stop()
	logging.Info("👋 Сервер успешно остановлен")
}
