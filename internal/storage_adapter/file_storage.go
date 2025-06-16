package storage_adapter

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
)

// BlockData представляет данные блока для хранения
type BlockData struct {
	ID       uint32                 `json:"id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// FileStorageAdapter реализует хранилище блоков в файловой системе
type FileStorageAdapter struct {
	basePath           string              // Базовый путь для хранения файлов
	chunkCache         map[vec.Vec2][]byte // Кеш чанков в памяти
	mu                 sync.RWMutex        // Мьютекс для безопасного доступа
	autoSave           bool                // Автоматическое сохранение изменений
	compressionEnabled bool                // Включить сжатие данных
}

// ChunkData представляет данные чанка для сериализации
type ChunkData struct {
	ChunkCoords  vec.Vec2                          `json:"chunk_coords"`
	Blocks       map[string]uint32                 `json:"blocks"`   // "x,y" -> blockID
	Metadata     map[string]map[string]interface{} `json:"metadata"` // "x,y" -> metadata
	Version      uint64                            `json:"version"`
	LastModified int64                             `json:"last_modified"`
}

// NewFileStorageAdapter создаёт новый файловый адаптер хранилища
func NewFileStorageAdapter(basePath string, autoSave bool) (*FileStorageAdapter, error) {
	// Создаём директорию если её нет
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("не удалось создать директорию %s: %w", basePath, err)
	}

	return &FileStorageAdapter{
		basePath:           basePath,
		chunkCache:         make(map[vec.Vec2][]byte),
		autoSave:           autoSave,
		compressionEnabled: false,
	}, nil
}

// LoadBlock загружает блок из хранилища
func (fsa *FileStorageAdapter) LoadBlock(pos vec.Vec2) (BlockData, error) {
	chunkCoords := pos.ToChunkCoords()

	fsa.mu.RLock()
	cachedData, exists := fsa.chunkCache[chunkCoords]
	fsa.mu.RUnlock()

	var chunkData ChunkData

	if exists {
		// Загружаем из кеша
		if err := json.Unmarshal(cachedData, &chunkData); err != nil {
			return BlockData{}, fmt.Errorf("ошибка десериализации кеша чанка %v: %w", chunkCoords, err)
		}
	} else {
		// Загружаем из файла
		filename := fsa.getChunkFilename(chunkCoords)
		data, err := os.ReadFile(filename)

		if os.IsNotExist(err) {
			// Чанк не существует, возвращаем пустой блок
			return BlockData{
				ID:       0,
				Metadata: make(map[string]interface{}),
			}, nil
		}

		if err != nil {
			return BlockData{}, fmt.Errorf("ошибка чтения файла чанка %s: %w", filename, err)
		}

		if err := json.Unmarshal(data, &chunkData); err != nil {
			return BlockData{}, fmt.Errorf("ошибка десериализации чанка %v: %w", chunkCoords, err)
		}

		// Кешируем данные
		fsa.mu.Lock()
		fsa.chunkCache[chunkCoords] = data
		fsa.mu.Unlock()
	}

	// Извлекаем блок из чанка
	localPos := pos.LocalInChunk()
	blockKey := fmt.Sprintf("%d,%d", localPos.X, localPos.Y)

	blockID, exists := chunkData.Blocks[blockKey]
	if !exists {
		blockID = 0 // Воздух
	}

	metadata, exists := chunkData.Metadata[blockKey]
	if !exists {
		metadata = make(map[string]interface{})
	}

	return BlockData{
		ID:       blockID,
		Metadata: metadata,
	}, nil
}

// SaveBlock сохраняет блок в хранилище
func (fsa *FileStorageAdapter) SaveBlock(pos vec.Vec2, block BlockData) error {
	chunkCoords := pos.ToChunkCoords()

	// Загружаем существующий чанк или создаём новый
	var chunkData ChunkData

	fsa.mu.RLock()
	cachedData, exists := fsa.chunkCache[chunkCoords]
	fsa.mu.RUnlock()

	if exists {
		if err := json.Unmarshal(cachedData, &chunkData); err != nil {
			return fmt.Errorf("ошибка десериализации кеша чанка %v: %w", chunkCoords, err)
		}
	} else {
		// Пытаемся загрузить из файла
		filename := fsa.getChunkFilename(chunkCoords)
		data, err := os.ReadFile(filename)

		if os.IsNotExist(err) {
			// Создаём новый чанк
			chunkData = ChunkData{
				ChunkCoords: chunkCoords,
				Blocks:      make(map[string]uint32),
				Metadata:    make(map[string]map[string]interface{}),
				Version:     1,
			}
		} else if err != nil {
			return fmt.Errorf("ошибка чтения файла чанка %s: %w", filename, err)
		} else {
			if err := json.Unmarshal(data, &chunkData); err != nil {
				return fmt.Errorf("ошибка десериализации чанка %v: %w", chunkCoords, err)
			}
		}
	}

	// Обновляем блок в чанке
	localPos := pos.LocalInChunk()
	blockKey := fmt.Sprintf("%d,%d", localPos.X, localPos.Y)

	if block.ID == 0 {
		// Удаляем блок (воздух)
		delete(chunkData.Blocks, blockKey)
		delete(chunkData.Metadata, blockKey)
	} else {
		chunkData.Blocks[blockKey] = block.ID
		if len(block.Metadata) > 0 {
			chunkData.Metadata[blockKey] = block.Metadata
		} else {
			delete(chunkData.Metadata, blockKey)
		}
	}

	chunkData.Version++
	chunkData.LastModified = time.Now().Unix()

	// Сериализуем обновлённый чанк
	data, err := json.Marshal(chunkData)
	if err != nil {
		return fmt.Errorf("ошибка сериализации чанка %v: %w", chunkCoords, err)
	}

	// Обновляем кеш
	fsa.mu.Lock()
	fsa.chunkCache[chunkCoords] = data
	fsa.mu.Unlock()

	// Сохраняем в файл если включено автосохранение
	if fsa.autoSave {
		return fsa.saveChunkToFile(chunkCoords, data)
	}

	return nil
}

// DeleteBlock удаляет блок из хранилища
func (fsa *FileStorageAdapter) DeleteBlock(pos vec.Vec2) error {
	return fsa.SaveBlock(pos, BlockData{
		ID:       0,
		Metadata: make(map[string]interface{}),
	})
}

// LoadChunk загружает весь чанк
func (fsa *FileStorageAdapter) LoadChunk(chunkCoords vec.Vec2) ([]BlockData, error) {
	filename := fsa.getChunkFilename(chunkCoords)
	data, err := os.ReadFile(filename)

	if os.IsNotExist(err) {
		// Возвращаем пустой чанк
		result := make([]BlockData, 16*16)
		for i := range result {
			result[i] = BlockData{
				ID:       0,
				Metadata: make(map[string]interface{}),
			}
		}
		return result, nil
	}

	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла чанка %s: %w", filename, err)
	}

	var chunkData ChunkData
	if err := json.Unmarshal(data, &chunkData); err != nil {
		return nil, fmt.Errorf("ошибка десериализации чанка %v: %w", chunkCoords, err)
	}

	// Конвертируем в массив блоков
	result := make([]BlockData, 16*16)

	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			blockKey := fmt.Sprintf("%d,%d", x, y)
			idx := y*16 + x

			blockID, exists := chunkData.Blocks[blockKey]
			if !exists {
				blockID = 0
			}

			metadata, exists := chunkData.Metadata[blockKey]
			if !exists {
				metadata = make(map[string]interface{})
			}

			result[idx] = BlockData{
				ID:       blockID,
				Metadata: metadata,
			}
		}
	}

	return result, nil
}

// SaveChunk сохраняет весь чанк
func (fsa *FileStorageAdapter) SaveChunk(chunkCoords vec.Vec2, blocks []BlockData) error {
	if len(blocks) != 16*16 {
		return fmt.Errorf("неверный размер чанка: ожидается %d блоков, получено %d", 16*16, len(blocks))
	}

	chunkData := ChunkData{
		ChunkCoords:  chunkCoords,
		Blocks:       make(map[string]uint32),
		Metadata:     make(map[string]map[string]interface{}),
		Version:      1,
		LastModified: time.Now().Unix(),
	}

	// Конвертируем массив блоков в мапу
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			idx := y*16 + x
			block := blocks[idx]

			if block.ID == 0 {
				continue // Пропускаем воздух
			}

			blockKey := fmt.Sprintf("%d,%d", x, y)
			chunkData.Blocks[blockKey] = block.ID

			if len(block.Metadata) > 0 {
				chunkData.Metadata[blockKey] = block.Metadata
			}
		}
	}

	// Сериализуем
	data, err := json.Marshal(chunkData)
	if err != nil {
		return fmt.Errorf("ошибка сериализации чанка %v: %w", chunkCoords, err)
	}

	// Обновляем кеш
	fsa.mu.Lock()
	fsa.chunkCache[chunkCoords] = data
	fsa.mu.Unlock()

	// Сохраняем в файл
	return fsa.saveChunkToFile(chunkCoords, data)
}

// FlushCache принудительно сохраняет все закешированные чанки
func (fsa *FileStorageAdapter) FlushCache() error {
	fsa.mu.RLock()
	chunks := make(map[vec.Vec2][]byte)
	for coords, data := range fsa.chunkCache {
		chunks[coords] = data
	}
	fsa.mu.RUnlock()

	for coords, data := range chunks {
		if err := fsa.saveChunkToFile(coords, data); err != nil {
			return fmt.Errorf("ошибка сохранения чанка %v: %w", coords, err)
		}
	}

	return nil
}

// GetStorageStats возвращает статистику хранилища
func (fsa *FileStorageAdapter) GetStorageStats() map[string]interface{} {
	fsa.mu.RLock()
	cachedChunks := len(fsa.chunkCache)
	fsa.mu.RUnlock()

	// Подсчитываем файлы в директории
	var fileCount int
	filepath.WalkDir(fsa.basePath, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".json" {
			fileCount++
		}
		return nil
	})

	return map[string]interface{}{
		"cached_chunks":       cachedChunks,
		"stored_files":        fileCount,
		"base_path":           fsa.basePath,
		"auto_save":           fsa.autoSave,
		"compression_enabled": fsa.compressionEnabled,
	}
}

// getChunkFilename возвращает имя файла для чанка
func (fsa *FileStorageAdapter) getChunkFilename(chunkCoords vec.Vec2) string {
	return filepath.Join(fsa.basePath, fmt.Sprintf("chunk_%d_%d.json", chunkCoords.X, chunkCoords.Y))
}

// saveChunkToFile сохраняет данные чанка в файл
func (fsa *FileStorageAdapter) saveChunkToFile(chunkCoords vec.Vec2, data []byte) error {
	filename := fsa.getChunkFilename(chunkCoords)

	// Создаём директорию если нужно
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию %s: %w", dir, err)
	}

	// Сохраняем файл
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("ошибка записи файла %s: %w", filename, err)
	}

	return nil
}
