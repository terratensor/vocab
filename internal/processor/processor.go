package processor

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// FileProcessor определяет интерфейс для обработки файлов.
type FileProcessor interface {
	// Process возвращает io.Reader для построчного чтения содержимого файла.
	Process(reader io.ReadSeeker) (io.Reader, error)
}

// NewProcessor создает процессор на основе расширения файла.
func NewProcessor(filePath string) (FileProcessor, error) {
	ext := filepath.Ext(filePath)
	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	// Если файл в архиве .gz
	if ext == ".gz" {
		innerExt := filepath.Ext(baseName)
		innerProcessor, err := newInnerProcessor(innerExt)
		if err != nil {
			return nil, fmt.Errorf("unsupported inner file format: %s", innerExt)
		}
		return NewGzipProcessor(innerProcessor), nil
	}

	// Обычные файлы
	return newInnerProcessor(ext)
}

// newInnerProcessor создает процессор для файлов без учета .gz.
func newInnerProcessor(ext string) (FileProcessor, error) {
	switch ext {
	case ".txt", ".md":
		return NewTextProcessor(), nil
	case ".pdf":
		return NewPDFProcessor(), nil
	case ".docx":
		return NewDOCXProcessor(), nil
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}
}
