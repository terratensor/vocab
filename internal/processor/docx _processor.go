package processor

import (
	"bytes"
	"io"
	"os"

	"github.com/nguyenthenguyen/docx"
)

// DOCXProcessor обрабатывает DOCX-файлы.
type DOCXProcessor struct{}

func NewDOCXProcessor() *DOCXProcessor {
	return &DOCXProcessor{}
}

func (p *DOCXProcessor) Process(reader io.ReadSeeker) (io.Reader, error) {
	// Сохраняем временный файл
	tmpFile, err := os.CreateTemp("", "*.docx")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	_, err = io.Copy(tmpFile, reader)
	if err != nil {
		return nil, err
	}

	// Чтение DOCX
	doc, err := docx.ReadDocxFile(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	defer doc.Close()

	// Возвращаем содержимое как io.Reader
	return bytes.NewReader([]byte(doc.Editable().GetContent())), nil
}
