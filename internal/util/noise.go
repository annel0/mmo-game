package util

import (
	"github.com/aquilax/go-perlin"
)

var perlinNoise *perlin.Perlin

// InitPerlinNoise инициализирует генератор шума Перлина с указанным сидом
func InitPerlinNoise(seed int64) {
	alpha := 2.0  // Сглаживание шума
	beta := 2.0   // Частота шума
	n := int32(3) // Количество октав
	perlinNoise = perlin.NewPerlin(alpha, beta, n, seed)
}

// PerlinNoise2D возвращает значение шума Перлина для указанных координат (от 0 до 1)
func PerlinNoise2D(x, y float64, seed int64) float64 {
	// Если генератор не инициализирован или используется другой сид, инициализируем его
	if perlinNoise == nil {
		InitPerlinNoise(seed)
	}

	// Получаем значение шума (от -1 до 1)
	noise := perlinNoise.Noise2D(x, y)

	// Преобразуем в диапазон от 0 до 1
	return (noise + 1.0) / 2.0
}
