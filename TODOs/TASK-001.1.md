# TASK-001.1: Подготовка протокола для NetChannel

## Описание
Рефакторинг существующего протокола для поддержки NetChannel. Основная проблема - использование `Payload []byte` вместо типизированных сообщений, что усложняет диспетчеризацию и требует дополнительных аллокаций.

## Проблемы текущей реализации

1. **GameMessage с Payload []byte**
   ```protobuf
   message GameMessage {
     MessageType type = 1;
     bytes payload = 2;  // Требует ручной десериализации
   }
   ```
   - Двойная сериализация (сообщение → payload → GameMessage)
   - Невозможность использовать protobuf reflection
   - Сложность в отладке

2. **Несоответствие ID**
   - Entity использует `uint64 ID`
   - Player хранится как `map[string]*entity.Player`
   - Конвертация ID приводит к overhead

3. **Отсутствие сетевых метаданных**
   - Нет sequence numbers для ordering
   - Нет acknowledgment для reliability
   - Нет приоритетов для QoS

## План реализации

### Шаг 1: Новая структура GameMessage
```protobuf
message GameMessage {
  // Метаданные NetChannel
  uint32 sequence = 1;
  uint32 ack = 2;
  uint32 ack_bits = 3;  // Битовая маска для группового ACK
  uint32 flags = 4;     // Reliable, Ordered, Priority
  
  // Тип сообщения для быстрой диспетчеризации
  MessageType type = 5;
  
  // Типизированный payload
  oneof payload {
    AuthRequest auth_request = 10;
    AuthResponse auth_response = 11;
    PlayerUpdate player_update = 12;
    WorldUpdate world_update = 13;
    ChatMessage chat_message = 14;
    BlockUpdate block_update = 15;
    ChunkData chunk_data = 16;
    EntitySpawn entity_spawn = 17;
    EntityDespawn entity_despawn = 18;
    InputCommand input_command = 19;
    StateSnapshot state_snapshot = 20;
    // ... другие типы
  }
  
  // Опциональное сжатие
  CompressionType compression = 30;
  bytes compressed_payload = 31;  // Если compression != NONE
}
```

### Шаг 2: Унификация ID
**Вариант A: Всё на uint64**
- Преимущества: Эффективность, совместимость с Entity
- Недостатки: Нужна миграция существующего кода

**Вариант B: Всё на string**
- Преимущества: Гибкость, читаемость, UUID support
- Недостатки: Больше памяти, медленнее сравнение

**Рекомендация**: Вариант A с helper функциями:
```go
// ID конвертеры для обратной совместимости
func PlayerIDToString(id uint64) string {
    return fmt.Sprintf("player_%d", id)
}

func ParsePlayerID(s string) (uint64, error) {
    var id uint64
    _, err := fmt.Sscanf(s, "player_%d", &id)
    return id, err
}
```

### Шаг 3: Новые типы сообщений

1. **AckMessage** - для подтверждения доставки
```protobuf
message AckMessage {
  uint32 sequence = 1;
  uint32 received_bits = 2;  // Битовая маска полученных пакетов
}
```

2. **HeartbeatMessage** - для keep-alive и RTT
```protobuf
message HeartbeatMessage {
  int64 client_time = 1;
  int64 server_time = 2;
}
```

3. **ConnectionMessage** - для handshake
```protobuf
message ConnectionMessage {
  enum Type {
    CONNECT = 0;
    ACCEPT = 1;
    REJECT = 2;
    DISCONNECT = 3;
  }
  Type type = 1;
  uint32 protocol_version = 2;
  string reason = 3;
  map<string, string> metadata = 4;
}
```

## Миграция

### Фаза 1: Добавление новых полей
1. Добавить новые поля в существующие proto файлы
2. Регенерировать .pb.go файлы
3. Обновить тесты

### Фаза 2: Постепенный переход
1. Создать адаптеры old ↔ new формата
2. Поддержка обоих форматов параллельно
3. Логирование использования старого формата

### Фаза 3: Удаление старого кода
1. Перевести всех клиентов на новый формат
2. Удалить адаптеры
3. Cleanup неиспользуемого кода

## Тестирование

1. **Unit тесты**
   - Сериализация/десериализация всех типов
   - Конвертеры ID
   - Совместимость форматов

2. **Benchmarks**
   - Сравнение производительности old vs new
   - Память и аллокации
   - Скорость диспетчеризации

3. **Compatibility тесты**
   - Старый клиент → новый сервер
   - Новый клиент → старый сервер
   - Миграция "на лету"

## Файлы для изменения

- `internal/protocol/common.proto` - базовые типы
- `internal/protocol/game.proto` - игровые сообщения  
- `internal/protocol/auth.proto` - авторизация
- `internal/protocol/network.proto` - новый файл для NetChannel
- `scripts/generate_proto.sh` - скрипт генерации

## Риски

1. **Breaking changes**
   - Митигация: Версионирование протокола
   
2. **Увеличение размера сообщений**
   - Митигация: Опциональное сжатие
   
3. **Сложность миграции**
   - Митигация: Поэтапный переход

## Критерии завершения

- [ ] Новая структура GameMessage с oneof
- [ ] Все типы сообщений мигрированы
- [ ] ID унифицированы на uint64
- [ ] Тесты покрывают 90%+ кода
- [ ] Benchmarks показывают улучшение
- [ ] Документация обновлена

## Зависимости
- Нет внешних зависимостей

## Приоритет
P1 (Блокирует всю TASK-001)

## Сложность
3/5

## Оценка времени
8 часов

## Статус
New 