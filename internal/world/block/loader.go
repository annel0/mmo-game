package block

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/annel0/mmo-game/internal/vec"
)

// simpleBlockBehavior — поведение статического блока,
// параметры задаются через JSON-файл в assets/blocks.
// Поддерживает только базовые функции без тиков.

type simpleBlockBehavior struct {
	id   BlockID
	name string
}

func (b *simpleBlockBehavior) ID() BlockID                           { return b.id }
func (b *simpleBlockBehavior) Name() string                          { return b.name }
func (b *simpleBlockBehavior) NeedsTick() bool                       { return false }
func (b *simpleBlockBehavior) TickUpdate(api BlockAPI, pos vec.Vec2) {}
func (b *simpleBlockBehavior) OnPlace(api BlockAPI, pos vec.Vec2)    {}
func (b *simpleBlockBehavior) OnBreak(api BlockAPI, pos vec.Vec2)    {}
func (b *simpleBlockBehavior) CreateMetadata() Metadata              { return nil }
func (b *simpleBlockBehavior) HandleInteraction(action string, cur, act map[string]interface{}) (BlockID, map[string]interface{}, InteractionResult) {
	return b.id, cur, InteractionResult{Success: false, Message: "no interaction"}
}

// jsonBlockSpec описывает схему JSON файла.

type jsonBlockSpec struct {
	ID   uint16 `json:"id"`
	Name string `json:"name"`
	// Дополнительно можно добавить поля solid, hardness и т.д.
}

// LoadJSONBlocks сканирует каталог и регистрирует блоки.
func LoadJSONBlocks(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		dec := json.NewDecoder(file)
		var spec jsonBlockSpec
		if err := dec.Decode(&spec); err != nil {
			return fmt.Errorf("block json %s: %w", path, err)
		}
		id := BlockID(spec.ID)
		if IsValidBlockID(id) {
			return fmt.Errorf("duplicate block id %d in %s", spec.ID, path)
		}
		Register(id, &simpleBlockBehavior{id: id, name: spec.Name})
		return nil
	})
}
