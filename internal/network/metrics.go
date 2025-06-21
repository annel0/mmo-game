package network

import (
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
)

// NetworkMetrics содержит детальные метрики сетевой подсистемы
type NetworkMetrics struct {
	// Общие метрики
	TotalConnections  int64
	ActiveConnections int64
	TotalMessages     int64
	TotalBytes        int64

	// Метрики по типам каналов
	ChannelMetrics map[ChannelType]*ChannelTypeMetrics

	// Метрики по типам сообщений
	MessageMetrics map[string]*MessageTypeMetrics

	// RTT статистика
	RTTStats RTTStatistics

	// Пропускная способность
	Bandwidth BandwidthMetrics

	// Ошибки
	ErrorMetrics ErrorStatistics

	// Последнее обновление
	LastUpdate time.Time

	mu sync.RWMutex
}

// ChannelTypeMetrics метрики для конкретного типа канала
type ChannelTypeMetrics struct {
	ActiveChannels   int64
	MessagesSent     int64
	MessagesReceived int64
	BytesSent        int64
	BytesReceived    int64
	Errors           int64
	AvgRTT           time.Duration
	PacketLoss       float64
}

// MessageTypeMetrics метрики для конкретного типа сообщения
type MessageTypeMetrics struct {
	Count            int64
	TotalSize        int64
	AvgSize          float64
	CompressionRatio float64
	ProcessingTime   time.Duration
}

// RTTStatistics статистика RTT
type RTTStatistics struct {
	Min     time.Duration
	Max     time.Duration
	Avg     time.Duration
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
	Samples []time.Duration // Последние N измерений
}

// BandwidthMetrics метрики пропускной способности
type BandwidthMetrics struct {
	InboundBps  float64 // Байт в секунду входящий трафик
	OutboundBps float64 // Байт в секунду исходящий трафик
	InboundPps  float64 // Пакетов в секунду входящий
	OutboundPps float64 // Пакетов в секунду исходящий

	// История для расчёта скользящего среднего
	inboundHistory  []float64
	outboundHistory []float64
	lastUpdate      time.Time
}

// ErrorStatistics статистика ошибок
type ErrorStatistics struct {
	ConnectionErrors    int64
	SerializationErrors int64
	TimeoutErrors       int64
	CompressionErrors   int64
	OtherErrors         int64

	// Последние ошибки для анализа
	RecentErrors []ErrorRecord
}

// ErrorRecord запись об ошибке
type ErrorRecord struct {
	Timestamp time.Time
	Type      string
	Message   string
	Channel   string
}

// NewNetworkMetrics создаёт новую систему метрик
func NewNetworkMetrics() *NetworkMetrics {
	return &NetworkMetrics{
		ChannelMetrics: make(map[ChannelType]*ChannelTypeMetrics),
		MessageMetrics: make(map[string]*MessageTypeMetrics),
		RTTStats: RTTStatistics{
			Samples: make([]time.Duration, 0, 1000), // Последние 1000 измерений
		},
		ErrorMetrics: ErrorStatistics{
			RecentErrors: make([]ErrorRecord, 0, 100), // Последние 100 ошибок
		},
		Bandwidth: BandwidthMetrics{
			inboundHistory:  make([]float64, 0, 60), // Последние 60 секунд
			outboundHistory: make([]float64, 0, 60),
		},
		LastUpdate: time.Now(),
	}
}

// RecordMessage записывает метрики отправленного/полученного сообщения
func (nm *NetworkMetrics) RecordMessage(channelType ChannelType, msg *protocol.NetGameMessage, size int, isOutbound bool, processingTime time.Duration) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Обновляем общие метрики
	nm.TotalMessages++
	nm.TotalBytes += int64(size)

	// Обновляем метрики канала
	channelMetrics := nm.getOrCreateChannelMetrics(channelType)
	if isOutbound {
		channelMetrics.MessagesSent++
		channelMetrics.BytesSent += int64(size)
	} else {
		channelMetrics.MessagesReceived++
		channelMetrics.BytesReceived += int64(size)
	}

	// Обновляем метрики типа сообщения
	msgType := nm.getMessageType(msg)
	msgMetrics := nm.getOrCreateMessageMetrics(msgType)
	msgMetrics.Count++
	msgMetrics.TotalSize += int64(size)
	msgMetrics.AvgSize = float64(msgMetrics.TotalSize) / float64(msgMetrics.Count)
	msgMetrics.ProcessingTime = (msgMetrics.ProcessingTime + processingTime) / 2 // Скользящее среднее

	nm.LastUpdate = time.Now()
}

// RecordRTT записывает измерение RTT
func (nm *NetworkMetrics) RecordRTT(rtt time.Duration) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Добавляем в историю
	nm.RTTStats.Samples = append(nm.RTTStats.Samples, rtt)
	if len(nm.RTTStats.Samples) > 1000 {
		nm.RTTStats.Samples = nm.RTTStats.Samples[1:] // Удаляем старые
	}

	// Обновляем статистику
	nm.calculateRTTStats()
}

// RecordError записывает ошибку
func (nm *NetworkMetrics) RecordError(errorType, message, channel string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Обновляем счётчики
	switch errorType {
	case "connection":
		nm.ErrorMetrics.ConnectionErrors++
	case "serialization":
		nm.ErrorMetrics.SerializationErrors++
	case "timeout":
		nm.ErrorMetrics.TimeoutErrors++
	case "compression":
		nm.ErrorMetrics.CompressionErrors++
	default:
		nm.ErrorMetrics.OtherErrors++
	}

	// Добавляем в историю
	nm.ErrorMetrics.RecentErrors = append(nm.ErrorMetrics.RecentErrors, ErrorRecord{
		Timestamp: time.Now(),
		Type:      errorType,
		Message:   message,
		Channel:   channel,
	})

	// Ограничиваем размер истории
	if len(nm.ErrorMetrics.RecentErrors) > 100 {
		nm.ErrorMetrics.RecentErrors = nm.ErrorMetrics.RecentErrors[1:]
	}
}

// UpdateBandwidth обновляет метрики пропускной способности
func (nm *NetworkMetrics) UpdateBandwidth() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	now := time.Now()
	if nm.Bandwidth.lastUpdate.IsZero() {
		nm.Bandwidth.lastUpdate = now
		return
	}

	// Рассчитываем скорость на основе накопленных данных
	duration := now.Sub(nm.Bandwidth.lastUpdate).Seconds()
	if duration > 0 {
		// Здесь должна быть логика расчёта на основе изменений в TotalBytes
		// Для упрощения используем заглушку
		inboundBps := float64(nm.TotalBytes) / duration
		outboundBps := float64(nm.TotalBytes) / duration

		// Добавляем в историю
		nm.Bandwidth.inboundHistory = append(nm.Bandwidth.inboundHistory, inboundBps)
		nm.Bandwidth.outboundHistory = append(nm.Bandwidth.outboundHistory, outboundBps)

		// Ограничиваем размер истории
		if len(nm.Bandwidth.inboundHistory) > 60 {
			nm.Bandwidth.inboundHistory = nm.Bandwidth.inboundHistory[1:]
			nm.Bandwidth.outboundHistory = nm.Bandwidth.outboundHistory[1:]
		}

		// Рассчитываем скользящее среднее
		nm.Bandwidth.InboundBps = nm.calculateAverage(nm.Bandwidth.inboundHistory)
		nm.Bandwidth.OutboundBps = nm.calculateAverage(nm.Bandwidth.outboundHistory)
	}

	nm.Bandwidth.lastUpdate = now
}

// GetSnapshot возвращает снимок текущих метрик
func (nm *NetworkMetrics) GetSnapshot() *NetworkMetrics {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	// Создаём новую структуру без мьютекса
	snapshot := &NetworkMetrics{
		TotalConnections:  nm.TotalConnections,
		ActiveConnections: nm.ActiveConnections,
		TotalMessages:     nm.TotalMessages,
		TotalBytes:        nm.TotalBytes,
		RTTStats:          nm.RTTStats,
		Bandwidth:         nm.Bandwidth,
		ErrorMetrics:      nm.ErrorMetrics,
		LastUpdate:        nm.LastUpdate,
	}

	// Копируем карты
	snapshot.ChannelMetrics = make(map[ChannelType]*ChannelTypeMetrics)
	for k, v := range nm.ChannelMetrics {
		channelCopy := *v
		snapshot.ChannelMetrics[k] = &channelCopy
	}

	snapshot.MessageMetrics = make(map[string]*MessageTypeMetrics)
	for k, v := range nm.MessageMetrics {
		msgCopy := *v
		snapshot.MessageMetrics[k] = &msgCopy
	}

	return snapshot
}

// Reset сбрасывает все метрики
func (nm *NetworkMetrics) Reset() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nm.TotalConnections = 0
	nm.ActiveConnections = 0
	nm.TotalMessages = 0
	nm.TotalBytes = 0

	nm.ChannelMetrics = make(map[ChannelType]*ChannelTypeMetrics)
	nm.MessageMetrics = make(map[string]*MessageTypeMetrics)

	nm.RTTStats = RTTStatistics{
		Samples: make([]time.Duration, 0, 1000),
	}

	nm.ErrorMetrics = ErrorStatistics{
		RecentErrors: make([]ErrorRecord, 0, 100),
	}

	nm.Bandwidth = BandwidthMetrics{
		inboundHistory:  make([]float64, 0, 60),
		outboundHistory: make([]float64, 0, 60),
	}

	nm.LastUpdate = time.Now()
}

// Приватные методы

func (nm *NetworkMetrics) getOrCreateChannelMetrics(channelType ChannelType) *ChannelTypeMetrics {
	if metrics, exists := nm.ChannelMetrics[channelType]; exists {
		return metrics
	}

	metrics := &ChannelTypeMetrics{}
	nm.ChannelMetrics[channelType] = metrics
	return metrics
}

func (nm *NetworkMetrics) getOrCreateMessageMetrics(msgType string) *MessageTypeMetrics {
	if metrics, exists := nm.MessageMetrics[msgType]; exists {
		return metrics
	}

	metrics := &MessageTypeMetrics{}
	nm.MessageMetrics[msgType] = metrics
	return metrics
}

func (nm *NetworkMetrics) getMessageType(msg *protocol.NetGameMessage) string {
	switch msg.Payload.(type) {
	case *protocol.NetGameMessage_AuthRequest:
		return "auth_request"
	case *protocol.NetGameMessage_AuthResponse:
		return "auth_response"
	case *protocol.NetGameMessage_ChunkRequest:
		return "chunk_request"
	case *protocol.NetGameMessage_ChunkData:
		return "chunk_data"
	case *protocol.NetGameMessage_BlockUpdate:
		return "block_update"
	case *protocol.NetGameMessage_EntitySpawn:
		return "entity_spawn"
	case *protocol.NetGameMessage_EntityMove:
		return "entity_move"
	case *protocol.NetGameMessage_Chat:
		return "chat"
	case *protocol.NetGameMessage_ChatBroadcast:
		return "chat_broadcast"
	case *protocol.NetGameMessage_Ping:
		return "ping"
	case *protocol.NetGameMessage_Pong:
		return "pong"
	default:
		return "unknown"
	}
}

func (nm *NetworkMetrics) calculateRTTStats() {
	if len(nm.RTTStats.Samples) == 0 {
		return
	}

	// Создаём копию для сортировки
	samples := make([]time.Duration, len(nm.RTTStats.Samples))
	copy(samples, nm.RTTStats.Samples)

	// Простая сортировка (можно оптимизировать)
	for i := 0; i < len(samples); i++ {
		for j := i + 1; j < len(samples); j++ {
			if samples[i] > samples[j] {
				samples[i], samples[j] = samples[j], samples[i]
			}
		}
	}

	// Рассчитываем статистику
	nm.RTTStats.Min = samples[0]
	nm.RTTStats.Max = samples[len(samples)-1]

	// Среднее
	var sum time.Duration
	for _, sample := range samples {
		sum += sample
	}
	nm.RTTStats.Avg = sum / time.Duration(len(samples))

	// Перцентили
	nm.RTTStats.P50 = samples[len(samples)*50/100]
	nm.RTTStats.P95 = samples[len(samples)*95/100]
	nm.RTTStats.P99 = samples[len(samples)*99/100]
}

func (nm *NetworkMetrics) calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
