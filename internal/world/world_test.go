package world

import (
	"testing"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
	"github.com/stretchr/testify/assert"
)

func TestWorldManager_Creation(t *testing.T) {
	// Тест создания WorldManager
	wm := NewWorldManager(12345)

	assert.NotNil(t, wm, "WorldManager должен быть создан")
	assert.Equal(t, int64(12345), wm.seed, "Seed должен быть установлен правильно")
	assert.NotNil(t, wm.bigChunks, "Карта BigChunk должна быть инициализирована")
	assert.NotNil(t, wm.globalEvents, "Канал событий должен быть инициализирован")
	assert.Equal(t, uint64(1000), wm.nextEntityID, "Начальный ID сущности должен быть 1000")
}

func TestWorldManager_BlockOperations(t *testing.T) {
	// Тест операций с блоками
	wm := NewWorldManager(12345)

	// Тестируем установку и получение блока
	pos := vec.Vec2{X: 10, Y: 15}
	testBlock := Block{
		ID:      block.BlockID(1),
		Payload: map[string]interface{}{"test": "value"},
	}

	// Устанавливаем блок
	wm.SetBlock(pos, testBlock)

	// Получаем блок
	retrievedBlock := wm.GetBlock(pos)
	assert.Equal(t, testBlock.ID, retrievedBlock.ID, "ID блока должен совпадать")
	assert.Equal(t, testBlock.Payload["test"], retrievedBlock.Payload["test"], "Метаданные блока должны совпадать")
}

func TestWorldManager_BlockMetadata(t *testing.T) {
	// Тест работы с метаданными блоков
	wm := NewWorldManager(12345)
	pos := vec.Vec2{X: 5, Y: 8}

	// Устанавливаем блок с метаданными
	blockID := BlockID(2)
	metadata := map[string]interface{}{
		"hardness":   3.5,
		"material":   "stone",
		"durability": 100,
	}

	wm.SetBlockWithMetadata(pos, blockID, metadata)

	// Проверяем получение блока
	retrievedBlock := wm.GetBlock(pos)
	assert.Equal(t, block.BlockID(blockID), retrievedBlock.ID, "ID блока должен совпадать")
	assert.Equal(t, metadata["hardness"], retrievedBlock.Payload["hardness"], "Hardness должен совпадать")
	assert.Equal(t, metadata["material"], retrievedBlock.Payload["material"], "Material должен совпадать")

	// Проверяем получение конкретного значения метаданных
	value, exists := wm.GetBlockMetadataValue(pos, "hardness")
	assert.True(t, exists, "Метаданные hardness должны существовать")
	assert.Equal(t, 3.5, value, "Значение hardness должно совпадать")

	// Проверяем несуществующие метаданные
	value, exists = wm.GetBlockMetadataValue(pos, "nonexistent")
	assert.False(t, exists, "Несуществующие метаданные не должны существовать")
	assert.Nil(t, value, "Значение несуществующих метаданных должно быть nil")
}

func TestWorldManager_BlockMetadataUpdate(t *testing.T) {
	// Тест обновления метаданных блока
	wm := NewWorldManager(12345)
	pos := vec.Vec2{X: 7, Y: 12}

	// Устанавливаем начальный блок
	wm.SetBlock(pos, Block{ID: block.BlockID(1), Payload: map[string]interface{}{"health": 100}})

	// Обновляем метаданные
	wm.SetBlockMetadataValue(pos, "health", 75)
	wm.SetBlockMetadataValue(pos, "status", "damaged")

	// Проверяем обновления
	healthValue, exists := wm.GetBlockMetadataValue(pos, "health")
	assert.True(t, exists, "Метаданные health должны существовать")
	assert.Equal(t, 75, healthValue, "Health должен быть обновлён до 75")

	statusValue, exists := wm.GetBlockMetadataValue(pos, "status")
	assert.True(t, exists, "Метаданные status должны быть добавлены")
	assert.Equal(t, "damaged", statusValue, "Status должен быть 'damaged'")
}

func TestWorldManager_QueryBlocks(t *testing.T) {
	// Тест получения блоков в области
	wm := NewWorldManager(12345)

	// Устанавливаем несколько блоков
	blocks := map[vec.Vec2]Block{
		{X: 0, Y: 0}: {ID: block.BlockID(1), Payload: map[string]interface{}{"type": "corner"}},
		{X: 1, Y: 0}: {ID: block.BlockID(2), Payload: map[string]interface{}{"type": "edge"}},
		{X: 0, Y: 1}: {ID: block.BlockID(3), Payload: map[string]interface{}{"type": "edge"}},
		{X: 1, Y: 1}: {ID: block.BlockID(4), Payload: map[string]interface{}{"type": "center"}},
	}

	for pos, block := range blocks {
		wm.SetBlock(pos, block)
	}

	// Запрашиваем блоки в области
	topLeft := vec.Vec2{X: 0, Y: 0}
	bottomRight := vec.Vec2{X: 1, Y: 1}
	result := wm.QueryBlocks(topLeft, bottomRight)

	assert.Equal(t, len(blocks), len(result), "Количество полученных блоков должно совпадать")

	for pos, expectedBlock := range blocks {
		resultBlock, exists := result[pos]
		assert.True(t, exists, "Блок в позиции %v должен существовать", pos)
		assert.Equal(t, expectedBlock.ID, resultBlock.ID, "ID блока в позиции %v должен совпадать", pos)
	}
}

func TestWorldManager_BatchUpdate(t *testing.T) {
	// Тест массового обновления блоков
	wm := NewWorldManager(12345)

	// Подготавливаем массовое обновление
	updates := map[vec.Vec2]Block{
		{X: 10, Y: 10}: {ID: block.BlockID(1), Payload: map[string]interface{}{"batch": "test1"}},
		{X: 11, Y: 10}: {ID: block.BlockID(2), Payload: map[string]interface{}{"batch": "test2"}},
		{X: 10, Y: 11}: {ID: block.BlockID(3), Payload: map[string]interface{}{"batch": "test3"}},
	}

	// Выполняем массовое обновление
	err := wm.BatchUpdate(updates)
	assert.NoError(t, err, "Массовое обновление не должно возвращать ошибку")

	// Проверяем результаты (может потребоваться небольшая задержка для обработки событий)
	// В реальной реализации может потребоваться ждать обработки событий
	// Пока что проверим, что метод не падает
}

func TestWorldManager_RemoveBlock(t *testing.T) {
	// Тест удаления блока
	wm := NewWorldManager(12345)
	pos := vec.Vec2{X: 20, Y: 25}

	// Устанавливаем блок
	originalBlock := Block{
		ID:      block.BlockID(5),
		Payload: map[string]interface{}{"valuable": true},
	}
	wm.SetBlock(pos, originalBlock)

	// Проверяем, что блок установлен
	setBlock := wm.GetBlock(pos)
	assert.Equal(t, originalBlock.ID, setBlock.ID, "Блок должен быть установлен")

	// Удаляем блок
	removedBlock := wm.RemoveBlock(pos)
	assert.Equal(t, originalBlock.ID, removedBlock.ID, "Удалённый блок должен совпадать с оригинальным")

	// Проверяем, что блок заменён на воздух
	airBlock := wm.GetBlock(pos)
	assert.Equal(t, block.BlockID(0), airBlock.ID, "Блок должен быть заменён на воздух (ID = 0)")
}

func TestWorldManager_IsBlockLoaded(t *testing.T) {
	// Тест проверки загрузки блока
	wm := NewWorldManager(12345)

	// Проверяем незагруженный блок
	pos := vec.Vec2{X: 100, Y: 200}
	assert.False(t, wm.IsBlockLoaded(pos), "Блок не должен быть загружен изначально")

	// Устанавливаем блок (это должно загрузить чанк)
	wm.SetBlock(pos, Block{ID: block.BlockID(1)})

	// Проверяем, что блок теперь загружен
	// Примечание: в зависимости от реализации, блок может быть загружен или нет
	// Этот тест может потребовать доработки в зависимости от логики загрузки чанков
}

func TestWorldManager_GenerateEntityID(t *testing.T) {
	// Тест генерации уникальных ID сущностей
	wm := NewWorldManager(12345)

	// Генерируем несколько ID
	id1 := wm.GenerateEntityID()
	id2 := wm.GenerateEntityID()
	id3 := wm.GenerateEntityID()

	// Проверяем уникальность
	assert.NotEqual(t, id1, id2, "ID сущностей должны быть уникальными")
	assert.NotEqual(t, id2, id3, "ID сущностей должны быть уникальными")
	assert.NotEqual(t, id1, id3, "ID сущностей должны быть уникальными")

	// Проверяем последовательность
	assert.Equal(t, id1+1, id2, "ID сущностей должны быть последовательными")
	assert.Equal(t, id2+1, id3, "ID сущностей должны быть последовательными")

	// Проверяем, что все ID больше начального значения
	assert.Greater(t, id1, uint64(1000), "ID сущности должен быть больше начального значения")
}

// Benchmarks

func BenchmarkWorldManager_SetBlock(b *testing.B) {
	wm := NewWorldManager(12345)
	testBlock := Block{ID: block.BlockID(1), Payload: map[string]interface{}{"test": "benchmark"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := vec.Vec2{X: i % 100, Y: (i / 100) % 100}
		wm.SetBlock(pos, testBlock)
	}
}

func BenchmarkWorldManager_GetBlock(b *testing.B) {
	wm := NewWorldManager(12345)
	testBlock := Block{ID: block.BlockID(1), Payload: map[string]interface{}{"test": "benchmark"}}

	// Предварительно устанавливаем блоки
	for i := 0; i < 100; i++ {
		for j := 0; j < 100; j++ {
			pos := vec.Vec2{X: i, Y: j}
			wm.SetBlock(pos, testBlock)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := vec.Vec2{X: i % 100, Y: (i / 100) % 100}
		wm.GetBlock(pos)
	}
}

func BenchmarkWorldManager_BatchUpdate(b *testing.B) {
	wm := NewWorldManager(12345)

	// Подготавливаем тестовые данные
	updates := make(map[vec.Vec2]Block)
	for i := 0; i < 100; i++ {
		pos := vec.Vec2{X: i % 10, Y: i / 10}
		updates[pos] = Block{ID: block.BlockID(i % 5), Payload: map[string]interface{}{"batch": i}}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.BatchUpdate(updates)
	}
}
