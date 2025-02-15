package processor

import (
	"compress/gzip"
	"io"
	"os"
)

// GzipProcessor обрабатывает файлы в формате .gz.
type GzipProcessor struct {
	innerProcessor FileProcessor // Процессор для распакованного содержимого
}

func NewGzipProcessor(innerProcessor FileProcessor) *GzipProcessor {
	return &GzipProcessor{
		innerProcessor: innerProcessor,
	}
}

func (p *GzipProcessor) Process(reader io.ReadSeeker) (io.Reader, error) {
	// Распаковка .gz
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	// Создаем временный файл для распакованных данных
	tmpFile, err := os.CreateTemp("", "*.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	// Копируем распакованные данные во временный файл
	_, err = io.Copy(tmpFile, gzReader)
	if err != nil {
		return nil, err
	}

	// Перемотка временного файла в начало
	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Обработка распакованного содержимого
	return p.innerProcessor.Process(tmpFile)
}