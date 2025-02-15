package processor

import (
	"io"
)

// TextProcessor обрабатывает текстовые файлы.
type TextProcessor struct{}

func NewTextProcessor() *TextProcessor {
	return &TextProcessor{}
}

func (p *TextProcessor) Process(reader io.ReadSeeker) (io.Reader, error) {
	// Текстовые файлы уже поддерживают построчное чтение.
	return reader, nil
}
