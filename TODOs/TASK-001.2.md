# TASK-001.2: Базовая инфраструктура NetChannel

## Описание
Создание основных компонентов для работы NetChannel: интерфейсы, менеджер каналов, сериализация. Эта инфраструктура будет использоваться всеми реализациями транспорта (KCP, QUIC, custom UDP).

## Текущее состояние

✅ **Выполнено:**
- Базовый интерфейс NetChannel
- Определены флаги надёжности
- Структура для конфигурации и статистики

❌ **Требуется:**
- Channel Manager для управления жизненным циклом
- Message Serializer с оптимизациями
- Метрики и профилирование

## Детальный план

### 1. Channel Manager

```go
// ChannelManager управляет пулом NetChannel
type ChannelManager struct {
    // Пул каналов для переиспользования
    pool        *sync.Pool
    
    // Активные каналы
    channels    map[string]NetChannel
    channelsMu  sync.RWMutex
    
    // Конфигурация
    config      *ManagerConfig
    
    // Метрики
    metrics     *ChannelMetrics
    
    // Хуки жизненного цикла
    onConnect   func(id string, channel NetChannel)
    onDisconnect func(id string, reason error)
}

type ManagerConfig struct {
    MaxChannels      int           // Максимум одновременных подключений
    ChannelTimeout   time.Duration // Таймаут неактивности
    ReconnectDelay   time.Duration // Задержка переподключения
    MaxReconnects    int           // Максимум попыток
    PoolSize         int           // Размер пула каналов
    MetricsInterval  time.Duration // Интервал сбора метрик
}
```

**Функциональность:**
- Создание/удаление каналов с переиспользованием
- Автоматическое переподключение при разрывах
- Очистка неактивных соединений
- Сбор и агрегация метрик
- Graceful shutdown

### 2. Message Serializer

```go
// MessageSerializer оптимизированная сериализация
type MessageSerializer struct {
    // Пулы буферов для Zero-Copy
    bufferPool      *sync.Pool
    
    // Компрессоры
    compressors     map[CompressionType]Compressor
    
    // Кеш сериализованных сообщений
    cache           *lru.Cache
    
    // Статистика
    stats           SerializerStats
}

// SerializationOptions настройки сериализации
type SerializationOptions struct {
    Compression     CompressionType
    BatchSize       int      // Для батчинга мелких сообщений
    MaxMessageSize  int      // Лимит размера
    EnableCache     bool     // Кеширование частых сообщений
}
```

**Оптимизации:**
1. **Zero-Copy**: Переиспользование буферов
2. **Батчинг**: Группировка мелких сообщений
3. **Компрессия**: zstd для больших сообщений
4. **Кеширование**: LRU кеш для частых сообщений

### 3. Метрики и профилирование

```go
// ChannelMetrics метрики NetChannel
type ChannelMetrics struct {
    // Счётчики
    MessagesSent     atomic.Uint64
    MessagesReceived atomic.Uint64
    BytesSent        atomic.Uint64
    BytesReceived    atomic.Uint64
    
    // Гистограммы (используем HdrHistogram)
    RTTHistogram     *hdrhistogram.Histogram
    SendLatency      *hdrhistogram.Histogram
    ReceiveLatency   *hdrhistogram.Histogram
    MessageSize      *hdrhistogram.Histogram
    
    // Текущие значения
    ActiveChannels   atomic.Int32
    PendingMessages  atomic.Int32
    
    // Ошибки по типам
    Errors          map[string]*atomic.Uint64
}

// MetricsCollector собирает метрики
type MetricsCollector interface {
    RecordSend(channel string, size int, latency time.Duration)
    RecordReceive(channel string, size int)
    RecordError(channel string, err error)
    RecordRTT(channel string, rtt time.Duration)
    
    // Экспорт для Prometheus/Grafana
    Export() []Metric
}
```

### 4. Расширенный интерфейс NetChannel

```go
// NetChannel v2 с дополнительными методами
type NetChannel interface {
    // Базовые методы (уже есть)
    Send(msg *protocol.GameMessage, flags ChannelFlags) error
    Receive() (*protocol.GameMessage, error)
    Close() error
    
    // Новые методы
    SendBatch(msgs []*protocol.GameMessage, flags ChannelFlags) error
    ReceiveBatch(max int) ([]*protocol.GameMessage, error)
    
    // Управление состоянием
    Pause() error
    Resume() error
    Reset() error
    
    // QoS
    SetPriority(priority int) error
    SetBandwidthLimit(bps int) error
    
    // Диагностика
    GetDiagnostics() *ChannelDiagnostics
    RunHealthCheck() error
}

// ChannelDiagnostics подробная диагностика
type ChannelDiagnostics struct {
    State           ChannelState
    QueueSizes      QueueStats
    CongestionInfo  CongestionStats
    ErrorCounters   map[string]int
    LastError       error
    LastErrorTime   time.Time
}
```

## Реализация по компонентам

### Компонент 1: Channel Pool
```go
func newChannelPool(factory func() NetChannel) *sync.Pool {
    return &sync.Pool{
        New: func() interface{} {
            ch := factory()
            // Инициализация метрик, буферов
            return ch
        },
    }
}
```

### Компонент 2: Auto-reconnect
```go
func (cm *ChannelManager) maintainConnection(id string) {
    for attempt := 0; attempt < cm.config.MaxReconnects; attempt++ {
        channel := cm.channels[id]
        if err := channel.RunHealthCheck(); err != nil {
            // Переподключение
            newChannel := cm.reconnect(id)
            if newChannel != nil {
                cm.replaceChannel(id, newChannel)
                break
            }
        }
        time.Sleep(cm.config.ReconnectDelay)
    }
}
```

### Компонент 3: Message Batching
```go
func (ms *MessageSerializer) BatchMessages(msgs []*protocol.GameMessage) ([]byte, error) {
    if len(msgs) == 1 {
        return ms.Serialize(msgs[0])
    }
    
    // Создаём батч-сообщение
    batch := &protocol.BatchMessage{
        Messages: msgs,
        Count:    uint32(len(msgs)),
    }
    
    return ms.serializeWithCompression(batch)
}
```

## Тестирование

### Unit тесты
1. **ChannelManager**
   - Создание/удаление каналов
   - Переподключение при сбоях
   - Очистка по таймауту
   - Лимиты подключений

2. **MessageSerializer**
   - Сериализация всех типов
   - Батчинг сообщений
   - Компрессия/декомпрессия
   - Производительность кеша

3. **Metrics**
   - Корректность счётчиков
   - Точность гистограмм
   - Экспорт метрик

### Benchmarks
```go
// Целевые показатели
BenchmarkSerialization-8     1000000      1050 ns/op     256 B/op       2 allocs/op
BenchmarkBatching-8           500000      2890 ns/op     512 B/op       3 allocs/op
BenchmarkCompression-8        100000     15670 ns/op    1024 B/op       5 allocs/op
```

## Интеграция

1. **С существующим кодом**
   - Адаптер для GameHandlerPB
   - Совместимость с текущим protocol

2. **С мониторингом**
   - Prometheus метрики
   - Grafana дашборды
   - Алерты на аномалии

## Файлы

- `internal/network/channel_manager.go` - менеджер каналов
- `internal/network/message_serializer.go` - сериализация
- `internal/network/metrics.go` - сбор метрик
- `internal/network/channel_pool.go` - пул каналов
- `internal/network/compressor.go` - компрессия

## Зависимости

- `github.com/HdrHistogram/hdrhistogram-go` - для гистограмм
- `github.com/klauspost/compress/zstd` - компрессия
- `github.com/hashicorp/golang-lru` - LRU кеш

## Критерии завершения

- [ ] ChannelManager с auto-reconnect
- [ ] MessageSerializer с батчингом
- [ ] Метрики экспортируются в Prometheus
- [ ] Покрытие тестами > 90%
- [ ] Benchmarks в пределах целевых
- [ ] Документация API

## Приоритет
P1 (Базовый функционал для NetChannel)

## Сложность
2/5

## Оценка времени
4 часа

## Статус
Partially Done (интерфейс готов, остальное - нет) 