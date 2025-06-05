/** NEWFILE **/
package world

// BlockLayer определяет логический "этаж" внутри чанка.
// LayerActive (1) остаётся слоем по умолчанию и совместим
// со старым однослойным кодом.
//
// 0 – LayerFloor: земля/фундаменты;
// 1 – LayerActive: поверхность, по которой ходят сущности;
// 2 – LayerCeiling: крыши/надстройки.
// Можно расширять до MaxLayers при необходимости.

type BlockLayer uint8

const (
	LayerFloor  BlockLayer = iota
	LayerActive            // главный слой совместимости
	LayerCeiling

	MaxLayers // всегда последний: количество слоев
)
