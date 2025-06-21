package storage

import (
	"context"
	"testing"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
)

// TestMemoryPositionRepo тестирует in-memory репозиторий позиций
func TestMemoryPositionRepo(t *testing.T) {
	repo := NewMemoryPositionRepo()
	ctx := context.Background()

	t.Run("Save and Load", func(t *testing.T) {
		userID := uint64(123)
		expectedPos := vec.Vec3{X: 10, Y: 20, Z: 1}

		// Сохраняем позицию
		err := repo.Save(ctx, userID, expectedPos)
		if err != nil {
			t.Fatalf("Ошибка сохранения позиции: %v", err)
		}

		// Загружаем позицию
		actualPos, found, err := repo.Load(ctx, userID)
		if err != nil {
			t.Fatalf("Ошибка загрузки позиции: %v", err)
		}

		if !found {
			t.Fatal("Позиция не найдена")
		}

		if actualPos != expectedPos {
			t.Errorf("Неверная позиция: ожидалась %+v, получена %+v", expectedPos, actualPos)
		}
	})

	t.Run("Load Non-Existent User", func(t *testing.T) {
		userID := uint64(999)

		pos, found, err := repo.Load(ctx, userID)
		if err != nil {
			t.Fatalf("Ошибка при загрузке несуществующего пользователя: %v", err)
		}

		if found {
			t.Error("Позиция найдена для несуществующего пользователя")
		}

		if pos != (vec.Vec3{}) {
			t.Errorf("Ожидалась пустая позиция, получена: %+v", pos)
		}
	})

	t.Run("Update Position", func(t *testing.T) {
		userID := uint64(456)
		firstPos := vec.Vec3{X: 1, Y: 2, Z: 1}
		secondPos := vec.Vec3{X: 3, Y: 4, Z: 2}

		// Сохраняем первую позицию
		err := repo.Save(ctx, userID, firstPos)
		if err != nil {
			t.Fatalf("Ошибка сохранения первой позиции: %v", err)
		}

		// Обновляем позицию
		err = repo.Save(ctx, userID, secondPos)
		if err != nil {
			t.Fatalf("Ошибка обновления позиции: %v", err)
		}

		// Проверяем, что позиция обновлена
		actualPos, found, err := repo.Load(ctx, userID)
		if err != nil {
			t.Fatalf("Ошибка загрузки обновленной позиции: %v", err)
		}

		if !found {
			t.Fatal("Обновленная позиция не найдена")
		}

		if actualPos != secondPos {
			t.Errorf("Неверная обновленная позиция: ожидалась %+v, получена %+v", secondPos, actualPos)
		}
	})

	t.Run("Delete Position", func(t *testing.T) {
		userID := uint64(789)
		pos := vec.Vec3{X: 5, Y: 6, Z: 1}

		// Сохраняем позицию
		err := repo.Save(ctx, userID, pos)
		if err != nil {
			t.Fatalf("Ошибка сохранения позиции: %v", err)
		}

		// Удаляем позицию
		err = repo.Delete(ctx, userID)
		if err != nil {
			t.Fatalf("Ошибка удаления позиции: %v", err)
		}

		// Проверяем, что позиция удалена
		_, found, err := repo.Load(ctx, userID)
		if err != nil {
			t.Fatalf("Ошибка загрузки после удаления: %v", err)
		}

		if found {
			t.Error("Позиция найдена после удаления")
		}
	})

	t.Run("BatchSave", func(t *testing.T) {
		positions := map[uint64]vec.Vec3{
			100: {X: 10, Y: 11, Z: 1},
			200: {X: 20, Y: 21, Z: 2},
			300: {X: 30, Y: 31, Z: 1},
		}

		// Пакетное сохранение
		err := repo.BatchSave(ctx, positions)
		if err != nil {
			t.Fatalf("Ошибка пакетного сохранения: %v", err)
		}

		// Проверяем каждую позицию
		for userID, expectedPos := range positions {
			actualPos, found, err := repo.Load(ctx, userID)
			if err != nil {
				t.Fatalf("Ошибка загрузки позиции для пользователя %d: %v", userID, err)
			}

			if !found {
				t.Errorf("Позиция не найдена для пользователя %d", userID)
				continue
			}

			if actualPos != expectedPos {
				t.Errorf("Неверная позиция для пользователя %d: ожидалась %+v, получена %+v",
					userID, expectedPos, actualPos)
			}
		}
	})

	t.Run("Validation", func(t *testing.T) {
		// Тест недействительного userID
		err := repo.Save(ctx, 0, vec.Vec3{X: 1, Y: 1, Z: 1})
		if err == nil {
			t.Error("Ожидалась ошибка для недействительного userID")
		}

		// Тест недействительного layer
		err = repo.Save(ctx, 123, vec.Vec3{X: 1, Y: 1, Z: -1})
		if err == nil {
			t.Error("Ожидалась ошибка для недействительного layer")
		}

		err = repo.Save(ctx, 123, vec.Vec3{X: 1, Y: 1, Z: 256})
		if err == nil {
			t.Error("Ожидалась ошибка для недействительного layer")
		}
	})

	t.Run("Context Cancellation", func(t *testing.T) {
		// Создаем отмененный контекст
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		userID := uint64(555)
		pos := vec.Vec3{X: 1, Y: 1, Z: 1}

		// Операция должна вернуть ошибку отмены контекста
		err := repo.Save(canceledCtx, userID, pos)
		if err != context.Canceled {
			t.Errorf("Ожидалась ошибка отмены контекста, получена: %v", err)
		}
	})
}

// TestMemoryPositionRepoUtilityMethods тестирует вспомогательные методы
func TestMemoryPositionRepoUtilityMethods(t *testing.T) {
	repo := NewMemoryPositionRepo()
	ctx := context.Background()

	// Начальное состояние
	if repo.Count() != 0 {
		t.Errorf("Ожидалось 0 позиций, получено: %d", repo.Count())
	}

	positions := map[uint64]vec.Vec3{
		1: {X: 1, Y: 1, Z: 1},
		2: {X: 2, Y: 2, Z: 1},
		3: {X: 3, Y: 3, Z: 2},
	}

	// Добавляем позиции
	for userID, pos := range positions {
		err := repo.Save(ctx, userID, pos)
		if err != nil {
			t.Fatalf("Ошибка сохранения позиции для пользователя %d: %v", userID, err)
		}
	}

	// Проверяем количество
	if repo.Count() != len(positions) {
		t.Errorf("Ожидалось %d позиций, получено: %d", len(positions), repo.Count())
	}

	// Проверяем GetAllPositions
	allPositions := repo.GetAllPositions()
	if len(allPositions) != len(positions) {
		t.Errorf("Ожидалось %d позиций в GetAllPositions, получено: %d",
			len(positions), len(allPositions))
	}

	for userID, expectedPos := range positions {
		if actualPos, exists := allPositions[userID]; !exists {
			t.Errorf("Позиция для пользователя %d не найдена в GetAllPositions", userID)
		} else if actualPos != expectedPos {
			t.Errorf("Неверная позиция для пользователя %d: ожидалась %+v, получена %+v",
				userID, expectedPos, actualPos)
		}
	}

	// Тестируем Clear
	repo.Clear()
	if repo.Count() != 0 {
		t.Errorf("После Clear ожидалось 0 позиций, получено: %d", repo.Count())
	}

	if len(repo.GetAllPositions()) != 0 {
		t.Error("После Clear GetAllPositions должна возвращать пустую карту")
	}
}

// TestConcurrentAccess тестирует concurrent доступ к репозиторию
func TestConcurrentAccess(t *testing.T) {
	repo := NewMemoryPositionRepo()
	ctx := context.Background()

	const numGoroutines = 10
	const numOperations = 100

	// Канал для синхронизации
	done := make(chan bool, numGoroutines)

	// Запускаем несколько горутин для параллельных операций
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < numOperations; j++ {
				userID := uint64(goroutineID*numOperations + j + 1) // +1 чтобы избежать userID = 0
				pos := vec.Vec3{X: goroutineID, Y: j, Z: 1}

				// Сохраняем
				err := repo.Save(ctx, userID, pos)
				if err != nil {
					t.Errorf("Ошибка сохранения в горутине %d: %v", goroutineID, err)
					return
				}

				// Загружаем
				loadedPos, found, err := repo.Load(ctx, userID)
				if err != nil {
					t.Errorf("Ошибка загрузки в горутине %d: %v", goroutineID, err)
					return
				}

				if !found {
					t.Errorf("Позиция не найдена в горутине %d для пользователя %d",
						goroutineID, userID)
					return
				}

				if loadedPos != pos {
					t.Errorf("Неверная позиция в горутине %d: ожидалась %+v, получена %+v",
						goroutineID, pos, loadedPos)
					return
				}
			}
		}(i)
	}

	// Ждем завершения всех горутин
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Тест превысил таймаут")
		}
	}

	// Проверяем финальное состояние
	expectedCount := numGoroutines * numOperations
	actualCount := repo.Count()
	if actualCount != expectedCount {
		t.Errorf("Ожидалось %d позиций после concurrent теста, получено: %d",
			expectedCount, actualCount)
	}
}
