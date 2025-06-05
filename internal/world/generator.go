package world

import (
	"math/rand"

	"github.com/annel0/mmo-game/internal/util"
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// BiomeType представляет тип биома
type BiomeType int

const (
	BiomePlains BiomeType = iota
	BiomeMountains
	BiomeWater
)

// WorldGenerator генерирует ландшафт мира
type WorldGenerator struct {
	Seed          int64   // Сид для генерации шума
	NoiseScale    float64 // Масштаб основного шума (высота)
	BiomeScale    float64 // Масштаб шума биомов
	ForestDensity float64 // Плотность лесов (от 0 до 1)
}

// NewWorldGenerator создаёт новый генератор мира
func NewWorldGenerator(seed int64) *WorldGenerator {
	// Инициализируем генератор шума
	util.InitPerlinNoise(seed)

	return &WorldGenerator{
		Seed:          seed,
		NoiseScale:    0.05, // Настройка сглаженности ландшафта
		BiomeScale:    0.02, // Настройка размера биомов
		ForestDensity: 0.05, // 5% шанс появления деревьев на равнинах
	}
}

// GenerateChunk генерирует чанк по его координатам
func (wg *WorldGenerator) GenerateChunk(coords vec.Vec2) *Chunk {
	chunk := NewChunk(coords)

	// Создаем локальный генератор случайных чисел для детерминированности
	// Для каждого чанка создаем уникальный сид на основе глобального сида и координат
	chunkSeed := wg.Seed + int64(coords.X*31) + int64(coords.Y*17)
	rng := rand.New(rand.NewSource(chunkSeed))

	globalStartX := coords.X << 4 // chunkX * 16
	globalStartY := coords.Y << 4 // chunkY * 16

	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			globalX := globalStartX + x
			globalY := globalStartY + y

			// Координаты для шума (масштабированные)
			noiseX := float64(globalX) * wg.NoiseScale
			noiseY := float64(globalY) * wg.NoiseScale

			// Генерация высоты на основе шума Перлина
			height := util.PerlinNoise2D(noiseX, noiseY, wg.Seed)

			// Координаты для шума биомов (другой масштаб)
			biomeNoiseX := float64(globalX) * wg.BiomeScale
			biomeNoiseY := float64(globalY) * wg.BiomeScale

			// Генерация значения для определения биома
			biomeValue := util.PerlinNoise2D(biomeNoiseX, biomeNoiseY, wg.Seed+42)

			// Определяем биом на основе высоты и значения биома
			biome := wg.getBiomeType(height, biomeValue)

			// Определяем тип блока на основе биома
			blockID := wg.getBlockForBiome(biome, height, rng)

			// Устанавливаем блок
			localPos := vec.Vec2{X: x, Y: y}
			chunk.SetBlock(localPos, blockID)

			// Если это равнины и подходящее место, добавляем дерево с некоторой вероятностью
			if biome == BiomePlains && rng.Float64() < wg.ForestDensity {
				wg.placeTreeMetadata(chunk, localPos, rng)
			}

			// Инициализируем метаданные для блока, если нужно
			behavior, exists := block.Get(blockID)
			if exists && behavior.NeedsTick() {
				// Сохраняем в список тикаемых
				chunk.Tickable[localPos] = struct{}{}

				// Инициализируем метаданные, если они еще не установлены
				if _, hasMetadata := chunk.Metadata[localPos]; !hasMetadata {
					metadata := behavior.CreateMetadata()
					if len(metadata) > 0 {
						chunk.Metadata[localPos] = metadata
					}
				}
			}
		}
	}

	return chunk
}

// getBiomeType определяет тип биома на основе значений шума
func (wg *WorldGenerator) getBiomeType(height, biomeValue float64) BiomeType {
	// Водные биомы в низинах
	if height < 0.3 {
		return BiomeWater
	}

	// Горные биомы на возвышенностях
	if height > 0.7 {
		return BiomeMountains
	}

	// Равнины в среднем диапазоне высот
	return BiomePlains
}

// getBlockForBiome возвращает тип блока для указанного биома
func (wg *WorldGenerator) getBlockForBiome(biome BiomeType, height float64, rng *rand.Rand) block.BlockID {
	switch biome {
	case BiomePlains:
		return block.GrassBlockID
	case BiomeMountains:
		return block.StoneBlockID
	case BiomeWater:
		return block.WaterBlockID
	default:
		return block.GrassBlockID
	}
}

// placeTreeMetadata добавляет метаданные дерева к блоку
func (wg *WorldGenerator) placeTreeMetadata(chunk *Chunk, pos vec.Vec2, rng *rand.Rand) {
	// Добавляем метаданные, указывающие на наличие дерева
	treeHeight := 3 + rng.Intn(3) // Высота дерева 3-5 блоков
	chunk.SetBlockMetadataMap(pos, map[string]interface{}{
		"has_tree":    true,
		"tree_height": treeHeight,
		"tree_type":   "oak", // Можно добавить разные типы деревьев
	})
}

// GenerateChunk глобальная функция для обратной совместимости
func GenerateChunk(seed int64, chunkCoords vec.Vec2) *Chunk {
	generator := NewWorldGenerator(seed)
	return generator.GenerateChunk(chunkCoords)
}
