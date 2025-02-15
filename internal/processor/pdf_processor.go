package processor

import (
	"bytes"
	"io"

	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// PDFProcessor обрабатывает PDF-файлы.
type PDFProcessor struct{}

func NewPDFProcessor() *PDFProcessor {
	return &PDFProcessor{}
}

func (p *PDFProcessor) Process(reader io.ReadSeeker) (io.Reader, error) {
	// Чтение PDF
	pdfReader, err := model.NewPdfReader(reader)
	if err != nil {
		return nil, err
	}

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return nil, err
	}

	var text bytes.Buffer
	for i := 1; i <= numPages; i++ {
		page, err := pdfReader.GetPage(i)
		if err != nil {
			return nil, err
		}

		ex, err := extractor.New(page)
		if err != nil {
			return nil, err
		}

		pageText, err := ex.ExtractText()
		if err != nil {
			return nil, err
		}

		text.WriteString(pageText + "\n")
	}

	// Возвращаем содержимое как io.Reader
	return bytes.NewReader(text.Bytes()), nil
}
