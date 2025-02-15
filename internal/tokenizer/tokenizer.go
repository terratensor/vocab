package tokenizer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/terratensor/segment"
	"github.com/terratensor/vocab/internal/processor"
)

type Tokenizer struct {
	lowercase   bool
	filterPunct bool
}

func ensureErrorDir() error {
	err := os.MkdirAll("vocab_errors", os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create error directory: %v", err)
	}
	return nil
}

func NewTokenizer(lowercase, filterPunct bool) *Tokenizer {
	return &Tokenizer{
		lowercase:   lowercase,
		filterPunct: filterPunct,
	}
}

func (t *Tokenizer) ProcessFiles(dirPath string, maxGoroutines int, outputFile string, sortType string) error {

	// Создаем папку для ошибок
	if err := ensureErrorDir(); err != nil {
		return err
	}

	var vocab = make(map[string]int)
	var mutex sync.Mutex
	guard := make(chan struct{}, maxGoroutines)
	var wg sync.WaitGroup

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("error reading directory %s: %v", dirPath, err)
	}

	totalFiles := len(files)
	processedFiles := 0
	var progressMutex sync.Mutex

	for _, fileEntry := range files {
		if fileEntry.IsDir() {
			continue
		}

		wg.Add(1)
		go func(fileEntry os.DirEntry) {
			defer wg.Done()
			guard <- struct{}{}
			defer func() { <-guard }()

			filePath := filepath.Join(dirPath, fileEntry.Name())
			file, err := os.Open(filePath)
			if err != nil {
				fmt.Println("Error opening file:", err)
				handleError(filePath, err) // Обработка ошибки
				return
			}
			defer file.Close()

			// Выбор процессора
			processor, err := processor.NewProcessor(filePath)
			if err != nil {
				fmt.Println("Error creating processor:", err)
				handleError(filePath, err) // Обработка ошибки
				return
			}

			// Обработка файла
			contentReader, err := processor.Process(file)
			if err != nil {
				fmt.Println("Error processing file:", err)
				handleError(filePath, err) // Обработка ошибки
				return
			}

			localVocab := make(map[string]int)

			// Построчное чтение и токенизация
			scanner := bufio.NewScanner(contentReader)
			buf := make([]byte, 1<<20) // 1 МБ
			scanner.Buffer(buf, 1<<20) // Устанавливаем максимальный размер токенаров

			for scanner.Scan() {
				line := scanner.Text()
				// Токенизация строки
				tokens := segment.NewTokenizer().Tokenize(line)
				// Обработка токенов
				for _, token := range tokens {
					tokenText := token.Text
					if t.lowercase {
						tokenText = strings.ToLower(tokenText)
					}
					if t.filterPunct && isPunctuation(tokenText) {
						continue
					}
					localVocab[tokenText]++
				}
			}

			if err := scanner.Err(); err != nil {
				fmt.Println("Error reading file:", err)
				handleError(filePath, err) // Обработка ошибки
			}

			mutex.Lock()
			for token, count := range localVocab {
				vocab[token] += count
			}
			mutex.Unlock()

			// Обновление прогресса
			progressMutex.Lock()
			processedFiles++
			fmt.Printf("\rProgress: %d/%d files processed (%.2f%%)", processedFiles, totalFiles, float64(processedFiles)/float64(totalFiles)*100)
			progressMutex.Unlock()
		}(fileEntry)
	}

	wg.Wait()
	fmt.Println()

	// Сохранение словаря
	return t.saveVocabulary(vocab, outputFile, sortType)
}

func (t *Tokenizer) saveVocabulary(vocab map[string]int, outputFile string, sortType string) error {
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	if sortType != "" {
		var tokenFrequencies []struct {
			Token string
			Count int
		}
		for token, count := range vocab {
			tokenFrequencies = append(tokenFrequencies, struct {
				Token string
				Count int
			}{Token: token, Count: count})
		}

		switch sortType {
		case "freq":
			sort.Slice(tokenFrequencies, func(i, j int) bool {
				return tokenFrequencies[i].Count > tokenFrequencies[j].Count
			})
		case "alpha":
			sort.Slice(tokenFrequencies, func(i, j int) bool {
				return tokenFrequencies[i].Token < tokenFrequencies[j].Token
			})
		}

		for _, tf := range tokenFrequencies {
			file.WriteString(fmt.Sprintf("%s %d\n", tf.Token, tf.Count))
		}
	} else {
		for token, count := range vocab {
			file.WriteString(fmt.Sprintf("%s %d\n", token, count))
		}
	}

	return nil
}

func isPunctuation(token string) bool {
	for _, r := range token {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			return false
		}
	}
	return true
}

// handleError обрабатывает ошибки: логирует и копирует файл.
func handleError(filePath string, err error) {
	// Логируем ошибку
	if logErr := logError(filePath, err); logErr != nil {
		fmt.Println("Failed to log error:", logErr)
	}

	// Копируем файл с ошибкой
	if copyErr := copyErrorFile(filePath); copyErr != nil {
		fmt.Println("Failed to copy error file:", copyErr)
	}
}

func logError(filePath string, logErr error) error {
	// Открываем лог-файл для добавления записей
	logFile, err := os.OpenFile("vocab_errors/errors.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	defer logFile.Close()

	// Формируем запись для лога
	logEntry := fmt.Sprintf("[%s] File: %s, Error: %v\n", time.Now().Format(time.RFC3339), filePath, logErr)

	// Записываем ошибку в лог
	_, err = logFile.WriteString(logEntry)
	if err != nil {
		return fmt.Errorf("failed to write to log file: %v", err)
	}

	return nil
}

func copyErrorFile(filePath string) error {
	// Получаем имя файла
	fileName := filepath.Base(filePath)

	// Создаем путь для копии
	dstPath := filepath.Join("vocab_errors", fileName)

	// Открываем исходный файл
	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	// Создаем файл для копии
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer dstFile.Close()

	// Копируем данные
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	return nil
}
