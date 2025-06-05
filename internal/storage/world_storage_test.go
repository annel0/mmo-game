package storage

import (
	"os"
	"testing"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/annel0/mmo-game/internal/world/block"
)

func setupTestStorage(t *testing.T) (*WorldStorage, string) {
	// Создаем временную директорию для тестов
	tempDir, err := os.MkdirTemp("", "world-storage-test")
	if err != nil {
		t.Fatalf("Не удалось создать временную директорию: %v", err)
	}

	// Инициализируем хранилище
	storage, err := NewWorldStorage(tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Не удалось создать хранилище: %v", err)
	}

	return storage, tempDir
}

func cleanupTestStorage(storage *WorldStorage, tempDir string) {
	if storage != nil {
		storage.Close()
	}
	if tempDir != "" {
		os.RemoveAll(tempDir)
	}
}

func TestSaveAndLoadChunk(t *testing.T) {
	storage, tempDir := setupTestStorage(t)
	defer cleanupTestStorage(storage, tempDir)

	// Создаем тестовый чанк
	coords := vec.Vec2{X: 10, Y: 20}
	chunk := world.NewChunk(coords)

	// Добавляем блоки с метаданными
	pos1 := vec.Vec2{X: 5, Y: 5}
	chunk.SetBlock(pos1, block.WaterBlockID)
	chunk.SetBlockMetadata(pos1, "level", 7)

	pos2 := vec.Vec2{X: 8, Y: 3}
	chunk.SetBlock(pos2, block.GrassBlockID)
	chunk.SetBlockMetadata(pos2, "growth", 3)

	// Сохраняем чанк
	err := storage.SaveChunk(chunk)
	if err != nil {
		t.Fatalf("Ошибка сохранения чанка: %v", err)
	}

	// Создаем новый пустой чанк с теми же координатами
	newChunk := world.NewChunk(coords)

	// Загружаем дельту
	delta, err := storage.LoadChunk(coords)
	if err != nil {
		t.Fatalf("Ошибка загрузки дельты чанка: %v", err)
	}

	// Проверяем дельту
	if delta.Coords.X != coords.X || delta.Coords.Y != coords.Y {
		t.Errorf("Неверные координаты дельты: %v, ожидалось %v", delta.Coords, coords)
	}

	if len(delta.BlockDeltas) != 2 {
		t.Errorf("Неверное количество изменений: %d, ожидалось 2", len(delta.BlockDeltas))
	}

	// Применяем дельту
	err = storage.ApplyDeltaToChunk(newChunk, delta)
	if err != nil {
		t.Fatalf("Ошибка применения дельты: %v", err)
	}

	// Проверяем, что блоки загружены правильно
	blockID1 := newChunk.GetBlock(pos1)
	if blockID1 != block.WaterBlockID {
		t.Errorf("Неверный ID блока: %d, ожидался %d", blockID1, block.WaterBlockID)
	}

	level, exists := newChunk.GetBlockMetadataValue(pos1, "level")
	if !exists {
		t.Error("Метаданные 'level' не найдены")
	} else {
		levelVal, ok := level.(float64) // JSON десериализует числа как float64
		if ok {
			if int(levelVal) != 7 {
				t.Errorf("Неверное значение метаданных: %v (%.1f), ожидалось 7", level, levelVal)
			}
		} else {
			// Пробуем как int
			levelInt, ok := level.(int)
			if !ok {
				t.Errorf("Неверный тип метаданных: %T, ожидался float64 или int", level)
			} else if levelInt != 7 {
				t.Errorf("Неверное значение метаданных: %d, ожидалось 7", levelInt)
			}
		}
	}

	blockID2 := newChunk.GetBlock(pos2)
	if blockID2 != block.GrassBlockID {
		t.Errorf("Неверный ID блока: %d, ожидался %d", blockID2, block.GrassBlockID)
	}

	growth, exists := newChunk.GetBlockMetadataValue(pos2, "growth")
	if !exists {
		t.Error("Метаданные 'growth' не найдены")
	} else {
		growthVal, ok := growth.(float64) // JSON десериализует числа как float64
		if ok {
			if int(growthVal) != 3 {
				t.Errorf("Неверное значение метаданных: %v (%.1f), ожидалось 3", growth, growthVal)
			}
		} else {
			// Пробуем как int
			growthInt, ok := growth.(int)
			if !ok {
				t.Errorf("Неверный тип метаданных: %T, ожидался float64 или int", growth)
			} else if growthInt != 3 {
				t.Errorf("Неверное значение метаданных: %d, ожидалось 3", growthInt)
			}
		}
	}
}

func TestLoadNonExistentChunk(t *testing.T) {
	storage, tempDir := setupTestStorage(t)
	defer cleanupTestStorage(storage, tempDir)

	// Пытаемся загрузить несуществующий чанк
	coords := vec.Vec2{X: 99, Y: 99}
	delta, err := storage.LoadChunk(coords)

	// Ошибки не должно быть, просто пустая дельта
	if err != nil {
		t.Fatalf("Ошибка при загрузке несуществующего чанка: %v", err)
	}

	// Дельта должна содержать правильные координаты и быть пустой
	if delta.Coords.X != coords.X || delta.Coords.Y != coords.Y {
		t.Errorf("Неверные координаты дельты: %v, ожидалось %v", delta.Coords, coords)
	}

	if len(delta.BlockDeltas) != 0 {
		t.Errorf("Дельта должна быть пустой, найдено %d изменений", len(delta.BlockDeltas))
	}
}
