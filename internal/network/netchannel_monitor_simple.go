package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
)

// SimpleNetChannelMonitor упрощённая версия монитора NetChannel
type SimpleNetChannelMonitor struct {
	logger   *logging.Logger
	channels map[string]NetChannel
	mu       sync.RWMutex

	// События для отладки
	events  []SimpleEvent
	eventMu sync.RWMutex

	// HTTP сервер
	httpServer *http.Server
	webUIPort  int
}

// SimpleEvent упрощённое событие
type SimpleEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	ChannelID   string    `json:"channel_id"`
	Description string    `json:"description"`
}

// SimpleStatus упрощённый статус системы
type SimpleStatus struct {
	Timestamp      time.Time           `json:"timestamp"`
	TotalChannels  int                 `json:"total_channels"`
	ActiveChannels int                 `json:"active_channels"`
	Channels       []SimpleChannelInfo `json:"channels"`
	Events         []SimpleEvent       `json:"recent_events"`
}

// SimpleChannelInfo упрощённая информация о канале
type SimpleChannelInfo struct {
	ID        string `json:"id"`
	Connected bool   `json:"connected"`
	Type      string `json:"type"`
}

// NewSimpleNetChannelMonitor создаёт упрощённый монитор
func NewSimpleNetChannelMonitor(logger *logging.Logger, webUIPort int) *SimpleNetChannelMonitor {
	return &SimpleNetChannelMonitor{
		logger:    logger,
		channels:  make(map[string]NetChannel),
		events:    make([]SimpleEvent, 0, 100),
		webUIPort: webUIPort,
	}
}

// Start запускает монитор
func (m *SimpleNetChannelMonitor) Start(ctx context.Context) error {
	m.logger.Info("🔍 Запуск упрощённого мониторинга NetChannel на порту %d", m.webUIPort)

	if err := m.startWebUI(); err != nil {
		return fmt.Errorf("failed to start web UI: %w", err)
	}

	m.addEvent("system", "monitor", "Система мониторинга запущена")
	return nil
}

// Stop останавливает монитор
func (m *SimpleNetChannelMonitor) Stop() error {
	if m.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.httpServer.Shutdown(ctx)
	}
	return nil
}

// RegisterChannel регистрирует канал
func (m *SimpleNetChannelMonitor) RegisterChannel(id string, channel NetChannel) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.channels[id] = channel
	m.addEvent("channel", id, fmt.Sprintf("Канал зарегистрирован: %T", channel))
	m.logger.Info("📋 Канал зарегистрирован: %s", id)
}

// UnregisterChannel снимает канал с мониторинга
func (m *SimpleNetChannelMonitor) UnregisterChannel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.channels[id]; exists {
		delete(m.channels, id)
		m.addEvent("channel", id, "Канал снят с мониторинга")
		m.logger.Info("📋 Канал снят с мониторинга: %s", id)
	}
}

// LogMessage логирует сообщение
func (m *SimpleNetChannelMonitor) LogMessage(channelID string, msg *protocol.NetGameMessage, direction string, size int) {
	msgType := "UNKNOWN"
	switch msg.Payload.(type) {
	case *protocol.NetGameMessage_AuthRequest:
		msgType = "AUTH_REQUEST"
	case *protocol.NetGameMessage_AuthResponse:
		msgType = "AUTH_RESPONSE"
	case *protocol.NetGameMessage_ChunkData:
		msgType = "CHUNK_DATA"
	case *protocol.NetGameMessage_EntityMove:
		msgType = "ENTITY_MOVE"
	case *protocol.NetGameMessage_Heartbeat:
		msgType = "HEARTBEAT"
	}

	description := fmt.Sprintf("%s %s (%d bytes)", direction, msgType, size)
	m.addEvent("message", channelID, description)
}

// LogError логирует ошибку
func (m *SimpleNetChannelMonitor) LogError(channelID string, err error, context string) {
	description := fmt.Sprintf("Ошибка в %s: %v", context, err)
	m.addEvent("error", channelID, description)
}

// LogConnection логирует событие соединения
func (m *SimpleNetChannelMonitor) LogConnection(channelID string, event string, addr string) {
	description := fmt.Sprintf("Соединение %s: %s", event, addr)
	m.addEvent("connection", channelID, description)
}

// GetStatus возвращает текущий статус
func (m *SimpleNetChannelMonitor) GetStatus() SimpleStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := SimpleStatus{
		Timestamp:      time.Now(),
		TotalChannels:  len(m.channels),
		ActiveChannels: 0,
		Channels:       make([]SimpleChannelInfo, 0, len(m.channels)),
	}

	// Собираем информацию о каналах
	for id, channel := range m.channels {
		connected := channel.IsConnected()
		if connected {
			status.ActiveChannels++
		}

		status.Channels = append(status.Channels, SimpleChannelInfo{
			ID:        id,
			Connected: connected,
			Type:      fmt.Sprintf("%T", channel),
		})
	}

	// Добавляем последние события
	m.eventMu.RLock()
	eventCount := len(m.events)
	start := 0
	if eventCount > 20 {
		start = eventCount - 20
	}
	status.Events = make([]SimpleEvent, eventCount-start)
	copy(status.Events, m.events[start:])
	m.eventMu.RUnlock()

	return status
}

// addEvent добавляет событие
func (m *SimpleNetChannelMonitor) addEvent(eventType, channelID, description string) {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()

	event := SimpleEvent{
		Timestamp:   time.Now(),
		Type:        eventType,
		ChannelID:   channelID,
		Description: description,
	}

	m.events = append(m.events, event)

	// Ограничиваем размер истории
	if len(m.events) > 100 {
		m.events = m.events[1:]
	}
}

// startWebUI запускает веб-интерфейс
func (m *SimpleNetChannelMonitor) startWebUI() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/status", m.handleAPIStatus)
	mux.HandleFunc("/", m.handleWebUI)

	m.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", m.webUIPort),
		Handler: mux,
	}

	go func() {
		if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("Ошибка веб-сервера мониторинга: %v", err)
		}
	}()

	return nil
}

func (m *SimpleNetChannelMonitor) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	status := m.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (m *SimpleNetChannelMonitor) handleWebUI(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>NetChannel Monitor</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; border-bottom: 2px solid #007bff; padding-bottom: 10px; }
        .status-card { background: #e8f4fd; padding: 15px; margin: 10px 0; border-radius: 5px; border-left: 4px solid #007bff; }
        .connected { color: #28a745; }
        .disconnected { color: #dc3545; }
        table { border-collapse: collapse; width: 100%; margin-top: 10px; }
        th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
        th { background-color: #f8f9fa; font-weight: bold; }
        tr:nth-child(even) { background-color: #f8f9fa; }
        .refresh-btn { background: #007bff; color: white; padding: 12px 24px; border: none; border-radius: 5px; cursor: pointer; margin-bottom: 20px; }
        .refresh-btn:hover { background: #0056b3; }
        .timestamp { color: #666; font-size: 0.9em; }
        .event-type { padding: 2px 6px; border-radius: 3px; font-size: 0.8em; }
        .event-message { background: #e7f3ff; }
        .event-connection { background: #fff3cd; }
        .event-error { background: #f8d7da; }
        .event-system { background: #d4edda; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔍 NetChannel Monitor</h1>
        <button class="refresh-btn" onclick="loadStatus()">🔄 Обновить</button>
        
        <div class="status-card">
            <h3>Статус системы</h3>
            <div id="system-status">Загрузка...</div>
        </div>
        
        <h2>📡 Активные каналы</h2>
        <div id="channels">Загрузка...</div>
        
        <h2>📝 Последние события</h2>
        <div id="events">Загрузка...</div>
    </div>

    <script>
        function loadStatus() {
            fetch('/api/status')
                .then(response => response.json())
                .then(data => {
                    // Системный статус
                    document.getElementById('system-status').innerHTML = 
                        '<strong>Всего каналов:</strong> ' + data.total_channels + ' | ' +
                        '<strong>Активных:</strong> ' + data.active_channels + '<br>' +
                        '<strong>Последнее обновление:</strong> ' + new Date(data.timestamp).toLocaleString();
                    
                    // Каналы
                    let channelsHtml = '<table><tr><th>ID канала</th><th>Тип</th><th>Статус</th></tr>';
                    data.channels.forEach(channel => {
                        channelsHtml += '<tr>' +
                            '<td><strong>' + channel.id + '</strong></td>' +
                            '<td>' + channel.type + '</td>' +
                            '<td class="' + (channel.connected ? 'connected' : 'disconnected') + '">' +
                            (channel.connected ? '🟢 Подключен' : '🔴 Отключен') + '</td>' +
                            '</tr>';
                    });
                    channelsHtml += '</table>';
                    document.getElementById('channels').innerHTML = channelsHtml;
                    
                    // События
                    let eventsHtml = '<table><tr><th>Время</th><th>Тип</th><th>Канал</th><th>Описание</th></tr>';
                    data.events.slice(-15).reverse().forEach(event => {
                        eventsHtml += '<tr>' +
                            '<td class="timestamp">' + new Date(event.timestamp).toLocaleTimeString() + '</td>' +
                            '<td><span class="event-type event-' + event.type + '">' + event.type + '</span></td>' +
                            '<td>' + (event.channel_id || 'system') + '</td>' +
                            '<td>' + event.description + '</td>' +
                            '</tr>';
                    });
                    eventsHtml += '</table>';
                    document.getElementById('events').innerHTML = eventsHtml;
                })
                .catch(error => {
                    console.error('Ошибка загрузки:', error);
                    document.getElementById('system-status').innerHTML = '❌ Ошибка загрузки данных';
                });
        }
        
        // Загружаем при старте
        loadStatus();
        
        // Автообновление каждые 5 секунд
        setInterval(loadStatus, 5000);
    </script>
</body>
</html>
    `

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
