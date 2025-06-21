package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
)

// TCPChannel реализует NetChannel для TCP соединений
type TCPChannel struct {
	conn   net.Conn
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

	// Буферы
	sendBuffer chan *protocol.NetGameMessage
	recvBuffer chan *protocol.NetGameMessage

	// Sequence tracking для надёжности
	sendSequence    uint32
	receiveSequence uint32

	mu sync.RWMutex
}

// NewTCPChannelFromConfig создаёт новый TCP канал
func NewTCPChannelFromConfig(config *ChannelConfig, logger *logging.Logger) *TCPChannel {
	ctx, cancel := context.WithCancel(context.Background())

	return &TCPChannel{
		config:     config,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		sendBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
		recvBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
	}
}

// NewTCPChannelFromConn создаёт TCP канал из существующего соединения
func NewTCPChannelFromConn(conn net.Conn, config *ChannelConfig, logger *logging.Logger) *TCPChannel {
	ctx, cancel := context.WithCancel(context.Background())

	channel := &TCPChannel{
		conn:       conn,
		config:     config,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		sendBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
		recvBuffer: make(chan *protocol.NetGameMessage, config.BufferSize),
	}

	// Устанавливаем статистику
	channel.stats.Connected = true
	channel.stats.RemoteAddr = conn.RemoteAddr().String()
	channel.stats.LastActivity = time.Now()

	// Запускаем горутины для обработки
	channel.wg.Add(2)
	go channel.sendLoop()
	go channel.receiveLoop()

	// Уведомляем о подключении
	if channel.onConnect != nil {
		channel.onConnect()
	}

	logger.Info("TCP channel created from connection: addr=%s", conn.RemoteAddr().String())
	return channel
}

// Connect устанавливает соединение с сервером
func (tc *TCPChannel) Connect(ctx context.Context, addr string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.conn != nil {
		return fmt.Errorf("already connected")
	}

	// Устанавливаем соединение
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	tc.conn = conn
	tc.stats.Connected = true
	tc.stats.RemoteAddr = addr
	tc.stats.LastActivity = time.Now()

	// Запускаем горутины для обработки
	tc.wg.Add(2)
	go tc.sendLoop()
	go tc.receiveLoop()

	// Уведомляем о подключении
	if tc.onConnect != nil {
		tc.onConnect()
	}

	tc.logger.Info("TCP channel connected: addr=%s", addr)
	return nil
}

// Send отправляет сообщение
func (tc *TCPChannel) Send(ctx context.Context, msg *protocol.NetGameMessage, opts *SendOptions) error {
	if !tc.IsConnected() {
		return fmt.Errorf("not connected")
	}

	// Устанавливаем sequence number
	msg.Sequence = atomic.AddUint32(&tc.sendSequence, 1)

	// Применяем опции если указаны
	if opts != nil {
		msg.Flags = opts.Flags
		msg.Compression = opts.Compression
	}

	select {
	case tc.sendBuffer <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-tc.ctx.Done():
		return fmt.Errorf("channel closed")
	}
}

// Receive получает сообщение
func (tc *TCPChannel) Receive(ctx context.Context) (*protocol.NetGameMessage, error) {
	select {
	case msg := <-tc.recvBuffer:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-tc.ctx.Done():
		return nil, fmt.Errorf("channel closed")
	}
}

// Close закрывает канал
func (tc *TCPChannel) Close() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.cancel != nil {
		tc.cancel()
	}

	if tc.conn != nil {
		err := tc.conn.Close()
		tc.conn = nil
		tc.stats.Connected = false

		// Ждём завершения горутин
		tc.wg.Wait()

		// Уведомляем об отключении
		if tc.onDisconnect != nil {
			tc.onDisconnect(err)
		}

		tc.logger.Info("TCP channel closed")
		return err
	}

	return nil
}

// IsConnected проверяет состояние соединения
func (tc *TCPChannel) IsConnected() bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.stats.Connected && tc.conn != nil
}

// RemoteAddr возвращает адрес удалённого клиента
func (tc *TCPChannel) RemoteAddr() string {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.stats.RemoteAddr
}

// Stats возвращает статистику соединения
func (tc *TCPChannel) Stats() ConnectionStats {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.stats
}

// RTT возвращает задержку (для TCP всегда 0)
func (tc *TCPChannel) RTT() time.Duration {
	return 0
}

// SetBufferSize устанавливает размер буфера
func (tc *TCPChannel) SetBufferSize(size int) error {
	return fmt.Errorf("buffer size cannot be changed after creation")
}

// SetTimeout устанавливает таймаут для операций
func (tc *TCPChannel) SetTimeout(timeout time.Duration) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.conn != nil {
		return tc.conn.SetDeadline(time.Now().Add(timeout))
	}
	return fmt.Errorf("not connected")
}

// SetKeepAlive устанавливает keep-alive для TCP соединения
func (tc *TCPChannel) SetKeepAlive(interval time.Duration) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tcpConn, ok := tc.conn.(*net.TCPConn); ok {
		if err := tcpConn.SetKeepAlive(true); err != nil {
			return err
		}
		return tcpConn.SetKeepAlivePeriod(interval)
	}
	return fmt.Errorf("not a TCP connection")
}

// OnMessage устанавливает обработчик сообщений
func (tc *TCPChannel) OnMessage(handler func(*protocol.NetGameMessage)) error {
	tc.onMessage = handler
	return nil
}

// OnConnect устанавливает обработчик подключения
func (tc *TCPChannel) OnConnect(handler func()) error {
	tc.onConnect = handler
	return nil
}

// OnDisconnect устанавливает обработчик отключения
func (tc *TCPChannel) OnDisconnect(handler func(error)) error {
	tc.onDisconnect = handler
	return nil
}

// OnError устанавливает обработчик ошибок
func (tc *TCPChannel) OnError(handler func(error)) error {
	tc.onError = handler
	return nil
}

// sendLoop отправляет сообщения
func (tc *TCPChannel) sendLoop() {
	defer tc.wg.Done()

	for {
		select {
		case msg := <-tc.sendBuffer:
			if err := tc.sendMessage(msg); err != nil {
				tc.logger.Error("Failed to send message: %v", err)
				if tc.onError != nil {
					tc.onError(err)
				}
			}
		case <-tc.ctx.Done():
			return
		}
	}
}

// receiveLoop получает сообщения
func (tc *TCPChannel) receiveLoop() {
	defer tc.wg.Done()

	for {
		select {
		case <-tc.ctx.Done():
			return
		default:
		}

		msg, err := tc.receiveMessage()
		if err != nil {
			if err == io.EOF {
				tc.logger.Info("Connection closed by remote")
			} else {
				tc.logger.Error("Failed to receive message: %v", err)
			}
			if tc.onError != nil {
				tc.onError(err)
			}
			return
		}

		// Обновляем статистику
		tc.stats.LastActivity = time.Now()
		tc.stats.PacketsReceived++

		// Отправляем в буфер или обработчик
		if tc.onMessage != nil {
			tc.onMessage(msg)
		} else {
			select {
			case tc.recvBuffer <- msg:
			default:
				tc.logger.Warn("Receive buffer full, dropping message")
			}
		}
	}
}

// sendMessage отправляет одно сообщение
func (tc *TCPChannel) sendMessage(msg *protocol.NetGameMessage) error {
	// Сериализуем сообщение
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Отправляем размер сообщения (4 байта)
	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(data)))

	if _, err := tc.conn.Write(sizeBuf); err != nil {
		return fmt.Errorf("failed to write message size: %w", err)
	}

	// Отправляем данные
	if _, err := tc.conn.Write(data); err != nil {
		return fmt.Errorf("failed to write message data: %w", err)
	}

	// Обновляем статистику
	tc.stats.PacketsSent++
	tc.stats.BytesSent += uint64(len(data) + 4)

	return nil
}

// receiveMessage получает одно сообщение
func (tc *TCPChannel) receiveMessage() (*protocol.NetGameMessage, error) {
	// Читаем размер сообщения (4 байта)
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(tc.conn, sizeBuf); err != nil {
		return nil, err
	}

	messageSize := binary.LittleEndian.Uint32(sizeBuf)
	const maxMessageSize = 65536 // 64KB максимум
	if messageSize > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes", messageSize)
	}

	// Читаем данные сообщения
	data := make([]byte, messageSize)
	if _, err := io.ReadFull(tc.conn, data); err != nil {
		return nil, err
	}

	// Десериализуем сообщение
	var msg protocol.NetGameMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Обновляем статистику
	tc.stats.BytesReceived += uint64(len(data) + 4)

	return &msg, nil
}
