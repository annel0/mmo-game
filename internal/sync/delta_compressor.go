package sync

import (
	"bytes"
	"compress/gzip"
	"io"
)

// DeltaCompressor кодирует/декодирует изменения (Change) в компактный вид.
// На первом этапе используем passthrough-компрессию — просто возвращаем вход.
// Позже планируется алгоритм XOR + GZip/VarInt для блоков/энтити.

type DeltaCompressor interface {
	Compress(changes []Change) ([]byte, error)
	Decompress(payload []byte) ([]Change, error)
}

type passthroughCompressor struct{}

func NewPassthroughCompressor() DeltaCompressor { return &passthroughCompressor{} }

func (p *passthroughCompressor) Compress(changes []Change) ([]byte, error) {
	// очень простой формат: [len(uint32)] [data] ...
	buf := make([]byte, 0)
	for _, c := range changes {
		n := uint32(len(c.Data))
		buf = append(buf, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
		buf = append(buf, c.Data...)
	}
	return buf, nil
}

func (p *passthroughCompressor) Decompress(payload []byte) ([]Change, error) {
	var res []Change
	i := 0
	for i < len(payload) {
		if i+4 > len(payload) {
			break // corrupt, игнорируем хвост
		}
		n := uint32(payload[i])<<24 | uint32(payload[i+1])<<16 | uint32(payload[i+2])<<8 | uint32(payload[i+3])
		i += 4
		if i+int(n) > len(payload) {
			break
		}
		res = append(res, Change{Data: payload[i : i+int(n)]})
		i += int(n)
	}
	return res, nil
}

// smartCompressor применяет gzip к serialized changes для лучшего сжатия
type smartCompressor struct{}

func NewSmartCompressor() DeltaCompressor { return &smartCompressor{} }

func (s *smartCompressor) Compress(changes []Change) ([]byte, error) {
	// Сначала сериализуем как passthrough
	passthrough := &passthroughCompressor{}
	raw, err := passthrough.Compress(changes)
	if err != nil {
		return nil, err
	}

	// Затем gzip
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *smartCompressor) Decompress(payload []byte) ([]Change, error) {
	// Decompress gzip
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	raw, err := io.ReadAll(gz)
	if err != nil {
		return nil, err
	}

	// Затем passthrough decode
	passthrough := &passthroughCompressor{}
	return passthrough.Decompress(raw)
}
