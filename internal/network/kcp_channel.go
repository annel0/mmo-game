package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/xtaci/kcp-go/v5"
	"google.golang.org/protobuf/proto"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
)

// KCPChannel реализует NetChannel для KCP (надёжный UDP)
type KCPChannel struct {
	conn   *kcp.UDPSession
	config *ChannelConfig
	logger *logging.Logger

	// Статистика
	stats ConnectionStats

	// Обработчики событий
	onMessage    func(*protocol.NetGameMessage)
	onConnect    func()
	onDisconnect func(error)
	onError      func(error)

	// Контроль выполнения
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Сжатие
	compressor   *zstd.Encoder
	decompressor *zstd.Decoder

	// Буферы
	sendBuffer chan *protocol.NetGameMessage
	recvBuffer chan *protocol.NetGameMessage

	// Sequence tracking для надёжности
	sendSequence    uint32
	receiveSequence uint32
	ackBits         uint32

	mu sync.RWMutex
}

// NewKCPChannel создаёт новый KCP канал
func NewKCPChannel(config *ChannelConfig, logger *logging.Logger) *KCPChannel {
	ctx, cancel := context.WithCancel(context.Background())

	channel := &KCPChannel{
		config:     config,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		sendBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
		recvBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
	}

	// Инициализируем сжатие если требуется
	if config.CompressionType == protocol.CompressionType_ZSTD {
		var err error
		channel.compressor, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			logger.Error("Failed to create compressor: %v", err)
		}

		channel.decompressor, err = zstd.NewReader(nil)
		if err != nil {
			logger.Error("Failed to create decompressor: %v", err)
		}
	}

	return channel
}

// NewKCPChannelFromConn создаёт новый KCP канал из существующего соединения
func NewKCPChannelFromConn(conn *kcp.UDPSession, config *ChannelConfig, logger *logging.Logger) *KCPChannel {
	ctx, cancel := context.WithCancel(context.Background())

	channel := &KCPChannel{
		conn:       conn,
		config:     config,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		sendBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
		recvBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
	}

	// Инициализируем сжатие если требуется
	if config.CompressionType == protocol.CompressionType_ZSTD {
		var err error
		channel.compressor, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			logger.Error("Failed to create compressor: %v", err)
		}

		channel.decompressor, err = zstd.NewReader(nil)
		if err != nil {
			logger.Error("Failed to create decompressor: %v", err)
		}
	}

	// Настраиваем KCP параметры для игрового трафика
	conn.SetStreamMode(true)
	conn.SetWriteDelay(false)
	conn.SetNoDelay(1, 20, 2, 1) // Агрессивные настройки для игр
	conn.SetWindowSize(512, 512) // Увеличиваем окно для пропускной способности
	conn.SetMtu(1400)            // Стандартный MTU для интернета

	// Устанавливаем статистику
	channel.stats.Connected = true
	channel.stats.RemoteAddr = conn.RemoteAddr().String()
	channel.stats.LastActivity = time.Now()

	// Запускаем горутины для обработки
	channel.wg.Add(3)
	go channel.sendLoop()
	go channel.receiveLoop()
	go channel.statsLoop()

	// Уведомляем о подключении
	if channel.onConnect != nil {
		channel.onConnect()
	}

	logger.Info("KCP channel created from connection: addr=%s", conn.RemoteAddr().String())
	return channel
}

// Connect устанавливает соединение с сервером
func (kc *KCPChannel) Connect(ctx context.Context, addr string) error {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	if kc.conn != nil {
		return fmt.Errorf("already connected")
	}

	// Устанавливаем соединение
	conn, err := kcp.DialWithOptions(addr, nil, 10, 3)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	// Настраиваем KCP параметры для игрового трафика
	conn.SetStreamMode(true)
	conn.SetWriteDelay(false)
	conn.SetNoDelay(1, 20, 2, 1) // Агрессивные настройки для игр
	conn.SetWindowSize(512, 512) // Увеличиваем окно для пропускной способности
	conn.SetMtu(1400)            // Стандартный MTU для интернета

	kc.conn = conn
	kc.stats.Connected = true
	kc.stats.RemoteAddr = addr
	kc.stats.LastActivity = time.Now()

	// Запускаем горутины для обработки
	kc.wg.Add(3)
	go kc.sendLoop()
	go kc.receiveLoop()
	go kc.statsLoop()

	// Уведомляем о подключении
	if kc.onConnect != nil {
		kc.onConnect()
	}

	kc.logger.Info("KCP channel connected: addr=%s", addr)
	return nil
}

// Send отправляет сообщение
func (kc *KCPChannel) Send(ctx context.Context, msg *protocol.NetGameMessage, opts *SendOptions) error {
	if !kc.IsConnected() {
		return fmt.Errorf("not connected")
	}

	// Устанавливаем sequence number
	msg.Sequence = atomic.AddUint32(&kc.sendSequence, 1)
	msg.Ack = kc.receiveSequence
	msg.AckBits = kc.ackBits

	// Применяем опции если указаны
	if opts != nil {
		msg.Flags = opts.Flags
		msg.Compression = opts.Compression
	}

	select {
	case kc.sendBuffer <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-kc.ctx.Done():
		return fmt.Errorf("channel closed")
	}
}

// Receive получает сообщение
func (kc *KCPChannel) Receive(ctx context.Context) (*protocol.NetGameMessage, error) {
	select {
	case msg := <-kc.recvBuffer:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-kc.ctx.Done():
		return nil, fmt.Errorf("channel closed")
	}
}

// Close закрывает канал
func (kc *KCPChannel) Close() error {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	if kc.conn == nil {
		return nil
	}

	// Останавливаем горутины
	kc.cancel()

	// Закрываем соединение
	err := kc.conn.Close()
	kc.conn = nil
	kc.stats.Connected = false

	// Ждём завершения горутин
	kc.wg.Wait()

	// Уведомляем о отключении
	if kc.onDisconnect != nil {
		kc.onDisconnect(err)
	}

	kc.logger.Info("KCP channel closed")
	return err
}

// IsConnected проверяет состояние соединения
func (kc *KCPChannel) IsConnected() bool {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	return kc.conn != nil && kc.stats.Connected
}

// RemoteAddr возвращает адрес удалённого узла
func (kc *KCPChannel) RemoteAddr() string {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	return kc.stats.RemoteAddr
}

// Stats возвращает статистику соединения
func (kc *KCPChannel) Stats() ConnectionStats {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	return kc.stats
}

// RTT возвращает текущий RTT
func (kc *KCPChannel) RTT() time.Duration {
	kc.mu.RLock()
	defer kc.mu.RUnlock()
	return kc.stats.RTT
}

// SetBufferSize устанавливает размер буфера
func (kc *KCPChannel) SetBufferSize(size int) error {
	// Для KCP буфер устанавливается при создании
	return nil
}

// SetTimeout устанавливает таймаут
func (kc *KCPChannel) SetTimeout(timeout time.Duration) error {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	if kc.conn != nil {
		return kc.conn.SetDeadline(time.Now().Add(timeout))
	}
	return nil
}

// SetKeepAlive устанавливает интервал keep-alive
func (kc *KCPChannel) SetKeepAlive(interval time.Duration) error {
	kc.config.KeepAlive = interval
	return nil
}

// OnMessage устанавливает обработчик сообщений
func (kc *KCPChannel) OnMessage(handler func(*protocol.NetGameMessage)) error {
	kc.onMessage = handler
	return nil
}

// OnConnect устанавливает обработчик подключения
func (kc *KCPChannel) OnConnect(handler func()) error {
	kc.onConnect = handler
	return nil
}

// OnDisconnect устанавливает обработчик отключения
func (kc *KCPChannel) OnDisconnect(handler func(error)) error {
	kc.onDisconnect = handler
	return nil
}

// OnError устанавливает обработчик ошибок
func (kc *KCPChannel) OnError(handler func(error)) error {
	kc.onError = handler
	return nil
}

// sendLoop обрабатывает отправку сообщений
func (kc *KCPChannel) sendLoop() {
	defer kc.wg.Done()

	for {
		select {
		case msg := <-kc.sendBuffer:
			if err := kc.sendMessage(msg); err != nil {
				kc.logger.Error("Failed to send message: %v", err)
				if kc.onError != nil {
					kc.onError(err)
				}
			}
		case <-kc.ctx.Done():
			return
		}
	}
}

// receiveLoop обрабатывает получение сообщений
func (kc *KCPChannel) receiveLoop() {
	defer kc.wg.Done()

	buffer := make([]byte, 65536)

	for {
		select {
		case <-kc.ctx.Done():
			return
		default:
			// Устанавливаем таймаут чтения
			kc.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

			n, err := kc.conn.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Таймаут чтения - это нормально
				}
				if kc.onError != nil {
					kc.onError(err)
				}
				continue
			}

			msg, err := kc.deserializeMessage(buffer[:n])
			if err != nil {
				kc.logger.Error("Failed to deserialize message: %v", err)
				continue
			}

			// Обновляем статистику
			atomic.AddUint64(&kc.stats.PacketsReceived, 1)
			atomic.AddUint64(&kc.stats.BytesReceived, uint64(n))
			kc.stats.LastActivity = time.Now()

			// Обновляем sequence tracking
			kc.updateAckBits(msg.Sequence)

			// Отправляем сообщение в буфер или обработчик
			select {
			case kc.recvBuffer <- msg:
			default:
				kc.logger.Warn("Receive buffer full, dropping message")
			}

			if kc.onMessage != nil {
				kc.onMessage(msg)
			}
		}
	}
}

// statsLoop обновляет статистику соединения
func (kc *KCPChannel) statsLoop() {
	defer kc.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if kc.conn != nil {
				// Обновляем RTT из KCP статистики
				kc.mu.Lock()
				// KCP не предоставляет прямого доступа к RTT, используем эвристику
				if time.Since(kc.stats.LastActivity) > kc.config.KeepAlive*2 {
					kc.stats.Connected = false
				}
				kc.mu.Unlock()
			}
		case <-kc.ctx.Done():
			return
		}
	}
}

// sendMessage отправляет сериализованное сообщение
func (kc *KCPChannel) sendMessage(msg *protocol.NetGameMessage) error {
	data, err := kc.serializeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	kc.mu.RLock()
	conn := kc.conn
	kc.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection closed")
	}

	// Отправляем данные
	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Обновляем статистику
	atomic.AddUint64(&kc.stats.PacketsSent, 1)
	atomic.AddUint64(&kc.stats.BytesSent, uint64(len(data)))
	kc.stats.LastActivity = time.Now()

	return nil
}

// serializeMessage сериализует сообщение в байты
func (kc *KCPChannel) serializeMessage(msg *protocol.NetGameMessage) ([]byte, error) {
	// Сериализуем protobuf
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	// Применяем сжатие если нужно
	if msg.Compression == protocol.CompressionType_ZSTD && kc.compressor != nil {
		data = kc.compressor.EncodeAll(data, nil)
	}

	// Добавляем заголовок с длиной
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(len(data)))

	return append(header, data...), nil
}

// deserializeMessage десериализует сообщение из байтов
func (kc *KCPChannel) deserializeMessage(data []byte) (*protocol.NetGameMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("message too short")
	}

	// Читаем длину
	length := binary.LittleEndian.Uint32(data[:4])
	if uint32(len(data)-4) != length {
		return nil, fmt.Errorf("message length mismatch")
	}

	payload := data[4:]

	// Создаём временное сообщение чтобы узнать тип сжатия
	var tempMsg protocol.NetGameMessage
	if err := proto.Unmarshal(payload, &tempMsg); err == nil {
		// Если сжатие применялось, декомпрессируем
		if tempMsg.Compression == protocol.CompressionType_ZSTD && kc.decompressor != nil {
			decompressed, err := kc.decompressor.DecodeAll(payload, nil)
			if err != nil {
				return nil, fmt.Errorf("decompression failed: %w", err)
			}
			payload = decompressed
		}
	}

	// Десериализуем финальное сообщение
	var msg protocol.NetGameMessage
	if err := proto.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// updateAckBits обновляет битовую маску подтверждений
func (kc *KCPChannel) updateAckBits(sequence uint32) {
	if sequence > kc.receiveSequence {
		// Новое сообщение - сдвигаем биты
		shift := sequence - kc.receiveSequence
		if shift < 32 {
			kc.ackBits <<= shift
			kc.ackBits |= 1 << (shift - 1)
		} else {
			kc.ackBits = 0
		}
		kc.receiveSequence = sequence
	} else if sequence < kc.receiveSequence {
		// Старое сообщение - устанавливаем соответствующий бит
		diff := kc.receiveSequence - sequence
		if diff < 32 {
			kc.ackBits |= 1 << (diff - 1)
		}
	}
}
