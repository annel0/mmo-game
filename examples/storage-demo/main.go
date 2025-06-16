package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/annel0/mmo-game/internal/storage_adapter"
	"github.com/annel0/mmo-game/internal/vec"
)

func main() {
	log.Println("=== Демонстрация файлового хранилища блоков ===")

	// 1. Создаём временную директорию для демонстрации
	tempDir := filepath.Join(os.TempDir(), "mmo-storage-demo")
	defer os.RemoveAll(tempDir) // Очищаем после демонстрации

	log.Printf("Используем директорию: %s", tempDir)

	// 2. Создаём файловый адаптер хранилища
	storage, err := storage_adapter.NewFileStorageAdapter(tempDir, true)
	if err != nil {
		log.Fatalf("Ошибка создания хранилища: %v", err)
	}

	log.Println("Файловое хранилище создано")

	// 3. Создаём тестовые блоки
	blocks := []struct {
		pos  vec.Vec2
		data storage_adapter.BlockData
		name string
	}{
		{
			pos: vec.Vec2{X: 0, Y: 0},
			data: storage_adapter.BlockData{
				ID: 1,
				Metadata: map[string]interface{}{
					"type":     "stone",
					"hardness": 3.5,
				},
			},
			name: "Камень",
		},
		{
			pos: vec.Vec2{X: 15, Y: 15},
			data: storage_adapter.BlockData{
				ID: 2,
				Metadata: map[string]interface{}{
					"type":   "grass",
					"growth": 0.8,
					"seeds":  []string{"wheat", "carrot"},
				},
			},
			name: "Трава",
		},
		{
			pos: vec.Vec2{X: 16, Y: 16}, // Это будет в другом чанке
			data: storage_adapter.BlockData{
				ID: 3,
				Metadata: map[string]interface{}{
					"type":        "water",
					"temperature": 15.5,
					"flow_rate":   2.3,
				},
			},
			name: "Вода",
		},
		{
			pos: vec.Vec2{X: 5, Y: 10},
			data: storage_adapter.BlockData{
				ID: 4,
				Metadata: map[string]interface{}{
					"type":      "chest",
					"inventory": []string{"sword", "potion", "gold"},
					"locked":    true,
					"owner_id":  12345,
				},
			},
			name: "Сундук",
		},
	}

	// 4. Сохраняем блоки
	log.Println("\n--- Сохранение блоков ---")
	for _, block := range blocks {
		err := storage.SaveBlock(block.pos, block.data)
		if err != nil {
			log.Printf("Ошибка сохранения блока %s в позиции %v: %v", block.name, block.pos, err)
		} else {
			log.Printf("Сохранён блок %s в позиции %v (ID=%d)", block.name, block.pos, block.data.ID)
		}
	}

	// 5. Проверяем статистику хранилища
	log.Println("\n--- Статистика хранилища ---")
	stats := storage.GetStorageStats()
	for key, value := range stats {
		log.Printf("%s: %v", key, value)
	}

	// 6. Загружаем и проверяем блоки
	log.Println("\n--- Проверка загрузки блоков ---")
	for _, block := range blocks {
		loadedBlock, err := storage.LoadBlock(block.pos)
		if err != nil {
			log.Printf("Ошибка загрузки блока в позиции %v: %v", block.pos, err)
			continue
		}

		if loadedBlock.ID == block.data.ID {
			log.Printf("✓ Блок %s загружен корректно: ID=%d, метаданных=%d",
				block.name, loadedBlock.ID, len(loadedBlock.Metadata))

			// Проверяем некоторые метаданные
			for key, expectedValue := range block.data.Metadata {
				if actualValue, exists := loadedBlock.Metadata[key]; exists {
					if fmt.Sprintf("%v", actualValue) == fmt.Sprintf("%v", expectedValue) {
						log.Printf("  ✓ Метаданные '%s': %v", key, actualValue)
					} else {
						log.Printf("  ✗ Метаданные '%s': ожидается %v, получено %v", key, expectedValue, actualValue)
					}
				} else {
					log.Printf("  ✗ Метаданные '%s' отсутствуют", key)
				}
			}
		} else {
			log.Printf("✗ Блок %s загружен неверно: ожидается ID=%d, получено ID=%d",
				block.name, block.data.ID, loadedBlock.ID)
		}
	}

	// 7. Тестируем загрузку несуществующего блока
	log.Println("\n--- Тестирование несуществующего блока ---")
	emptyPos := vec.Vec2{X: 1000, Y: 1000}
	emptyBlock, err := storage.LoadBlock(emptyPos)
	if err != nil {
		log.Printf("Ошибка загрузки несуществующего блока: %v", err)
	} else {
		log.Printf("Несуществующий блок: ID=%d (должен быть 0)", emptyBlock.ID)
	}

	// 8. Тестируем загрузку целого чанка
	log.Println("\n--- Тестирование загрузки чанка ---")
	chunkCoords := vec.Vec2{X: 0, Y: 0}
	chunkBlocks, err := storage.LoadChunk(chunkCoords)
	if err != nil {
		log.Printf("Ошибка загрузки чанка %v: %v", chunkCoords, err)
	} else {
		nonEmptyBlocks := 0
		for i, block := range chunkBlocks {
			if block.ID != 0 {
				nonEmptyBlocks++
				x := i % 16
				y := i / 16
				log.Printf("Чанк [%d,%d]: блок ID=%d в локальной позиции (%d,%d)",
					chunkCoords.X, chunkCoords.Y, block.ID, x, y)
			}
		}
		log.Printf("В чанке %v найдено %d непустых блоков из %d", chunkCoords, nonEmptyBlocks, len(chunkBlocks))
	}

	// 9. Тестируем удаление блока
	log.Println("\n--- Тестирование удаления блока ---")
	deletePos := blocks[0].pos
	err = storage.DeleteBlock(deletePos)
	if err != nil {
		log.Printf("Ошибка удаления блока: %v", err)
	} else {
		log.Printf("Блок в позиции %v удалён", deletePos)

		// Проверяем, что блок действительно удален
		deletedBlock, err := storage.LoadBlock(deletePos)
		if err != nil {
			log.Printf("Ошибка проверки удалённого блока: %v", err)
		} else {
			log.Printf("Проверка удаления: блок ID=%d (должен быть 0)", deletedBlock.ID)
		}
	}

	// 10. Принудительное сохранение кеша
	log.Println("\n--- Принудительное сохранение кеша ---")
	err = storage.FlushCache()
	if err != nil {
		log.Printf("Ошибка сохранения кеша: %v", err)
	} else {
		log.Printf("Кеш сохранён")
	}

	// Финальная статистика
	log.Println("\n--- Финальная статистика ---")
	finalStats := storage.GetStorageStats()
	for key, value := range finalStats {
		log.Printf("%s: %v", key, value)
	}

	log.Println("\n=== Демонстрация файлового хранилища завершена ===")
}
