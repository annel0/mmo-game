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
	BiomeDesert
	BiomeForest
	BiomeMountains
	BiomeWater
	BiomeDeepWater
)

// Константы высот для генерации
const (
	DeepWaterMax    = 0.20 // Ниже - глубинная вода
	ShallowWaterMax = 0.30 // Ниже - мелководье
	ActiveStart     = 0.60 // Выше - активные блоки (трава, деревья)
	MountainStart   = 0.80 // Выше - горы с рудами
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

			// Генерируем блоки для слоев
			floorID, activeID := wg.getBlocksForHeight(height, biome, rng)

			localPos := vec.Vec2{X: x, Y: y}

			// Слой "пол" всегда заполняем
			chunk.SetBlockLayer(LayerFloor, localPos, floorID)
			// Слой Active – ландшафт/вода/т.п.
			chunk.SetBlockLayer(LayerActive, localPos, activeID)

			// Добавляем деревья и другие объекты
			if activeID == block.AirBlockID && floorID != block.WaterBlockID && floorID != block.DeepWaterBlockID {
				// На суше можем разместить объекты
				if biome == BiomeForest && rng.Float64() < 0.15 { // 15% шанс дерева в лесу
					chunk.SetBlockLayer(LayerActive, localPos, block.TreeBlockID)
					wg.placeTreeMetadata(chunk, localPos, rng)
				} else if biome == BiomePlains && rng.Float64() < wg.ForestDensity {
					chunk.SetBlockLayer(LayerActive, localPos, block.TreeBlockID)
					wg.placeTreeMetadata(chunk, localPos, rng)
				} else if biome == BiomeDesert && rng.Float64() < 0.02 { // 2% шанс кактуса в пустыне
					chunk.SetBlockLayer(LayerActive, localPos, block.CactusBlockID)
				}
			}

			// Инициализируем метаданные для блока active-слоя, если нужно
			activeBlockID := chunk.GetBlockLayer(LayerActive, localPos)
			behavior, exists := block.Get(activeBlockID)
			if exists && behavior.NeedsTick() {
				chunk.Tickable3D[BlockCoord{Layer: LayerActive, Pos: localPos}] = struct{}{}

				if _, has := chunk.Metadata3D[BlockCoord{Layer: LayerActive, Pos: localPos}]; !has {
					metadata := behavior.CreateMetadata()
					if len(metadata) > 0 {
						chunk.Metadata3D[BlockCoord{Layer: LayerActive, Pos: localPos}] = metadata
					}
				}
			}
		}
	}

	return chunk
}

// getBlocksForHeight возвращает блоки для слоев пола и активного в зависимости от высоты
func (wg *WorldGenerator) getBlocksForHeight(height float64, biome BiomeType, rng *rand.Rand) (floorID, activeID block.BlockID) {
	switch {
	case height < DeepWaterMax:
		// Глубинная вода
		floorID = block.DeepWaterBlockID
		activeID = block.AirBlockID

	case height < ShallowWaterMax:
		// Мелководье
		floorID = block.WaterBlockID
		activeID = block.AirBlockID

	case height < ActiveStart:
		// Равнины - только пол
		floorID = wg.getFloorBlockForBiome(biome)
		activeID = block.AirBlockID

	case height < MountainStart:
		// Холмы - пол + возможные активные блоки
		floorID = wg.getFloorBlockForBiome(biome)
		activeID = block.AirBlockID // Деревья и другие объекты добавляются отдельно

	default:
		// Горы
		floorID = block.StoneBlockID
		// С некоторой вероятностью генерируем руду
		if rng.Float64() < 0.1 { // 10% шанс руды
			activeID = block.StoneBlockID // Пока используем камень, позже добавим руды
		} else {
			activeID = block.AirBlockID
		}
	}

	return floorID, activeID
}

// getFloorBlockForBiome возвращает блок пола для указанного биома
func (wg *WorldGenerator) getFloorBlockForBiome(biome BiomeType) block.BlockID {
	switch biome {
	case BiomeDesert:
		return block.SandBlockID
	case BiomeMountains:
		return block.StoneBlockID
	default:
		return block.DirtBlockID
	}
}

// getBiomeType определяет тип биома на основе значений шума
func (wg *WorldGenerator) getBiomeType(height, biomeValue float64) BiomeType {
	// Водные биомы в низинах
	if height < DeepWaterMax {
		return BiomeDeepWater
	}
	if height < ShallowWaterMax {
		return BiomeWater
	}

	// Горные биомы на возвышенностях
	if height > MountainStart {
		return BiomeMountains
	}

	// Для средних высот выбираем биом на основе biomeValue
	if biomeValue < -0.3 {
		return BiomeDesert
	} else if biomeValue > 0.3 {
		return BiomeForest
	}

	return BiomePlains
}

// getBlockForBiome возвращает тип блока для указанного биома (устаревший метод, оставлен для совместимости)
func (wg *WorldGenerator) getBlockForBiome(biome BiomeType, height float64, rng *rand.Rand) block.BlockID {
	switch biome {
	case BiomePlains:
		return block.GrassBlockID
	case BiomeMountains:
		return block.StoneBlockID
	case BiomeWater:
		return block.WaterBlockID
	case BiomeDeepWater:
		return block.DeepWaterBlockID
	case BiomeDesert:
		return block.SandBlockID
	case BiomeForest:
		return block.GrassBlockID
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
