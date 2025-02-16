package tokenizer

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/terratensor/segment"
)

type Tokenizer struct {
	lowercase   bool
	filterPunct bool
	errorDir    string
	logFile     *os.File
}

func NewTokenizer(lowercase, filterPunct bool) (*Tokenizer, error) {
	// Создаем папку для ошибок
	errorDir := "vocab_errors"
	if err := os.MkdirAll(errorDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create error directory: %v", err)
	}

	// Создаем лог-файл
	logFile, err := os.OpenFile(filepath.Join(errorDir, "vocab_errors.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %v", err)
	}

	return &Tokenizer{
		lowercase:   lowercase,
		filterPunct: filterPunct,
		errorDir:    errorDir,
		logFile:     logFile,
	}, nil
}

func (t *Tokenizer) Close() {
	t.logFile.Close()
}

// Загрузка словаря из файла
func (t *Tokenizer) LoadVocabulary(filePath string) (map[string]int, error) {
	vocab := make(map[string]int)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening vocabulary file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			continue // Пропускаем некорректные строки
		}
		token := parts[0]
		count := 0
		fmt.Sscanf(parts[1], "%d", &count)
		vocab[token] = count
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading vocabulary file: %v", err)
	}

	return vocab, nil
}

// Объединение словарей из нескольких файлов
func (t *Tokenizer) MergeVocabularies(filePaths []string) (map[string]int, error) {
	mergedVocab := make(map[string]int)

	for _, filePath := range filePaths {
		vocab, err := t.LoadVocabulary(filePath)
		if err != nil {
			return nil, fmt.Errorf("error loading vocabulary from %s: %v", filePath, err)
		}

		// Объединяем словари
		for token, count := range vocab {
			mergedVocab[token] += count
		}
	}

	return mergedVocab, nil
}

// Обработка словаря (приведение к нижнему регистру, фильтрация пунктуации)
func (t *Tokenizer) ProcessVocabulary(vocab map[string]int) map[string]int {
	processedVocab := make(map[string]int)

	for token, count := range vocab {
		// Приведение к нижнему регистру
		if t.lowercase {
			token = strings.ToLower(token)
		}

		// Фильтрация пунктуации
		if t.filterPunct && isPunctuation(token) {
			continue
		}

		// Обновление словаря
		processedVocab[token] += count
	}

	return processedVocab
}

// Сохранение словаря в файл с учетом сортировки
func (t *Tokenizer) SaveVocabulary(vocab map[string]int, outputFile string, sortType string) error {
	file, err := os.Create(outputFile)
	if err != nil {
		t.logError(fmt.Sprintf("Error creating output file %s: %v", outputFile, err))
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	// Если сортировка не требуется, сохраняем словарь как есть
	if sortType == "" {
		for token, count := range vocab {
			file.WriteString(fmt.Sprintf("%s %d\n", token, count))
		}
		return nil
	}

	// Преобразуем словарь в слайс для сортировки
	type TokenFrequency struct {
		Token string
		Count int
	}
	var tokenFrequencies []TokenFrequency
	for token, count := range vocab {
		tokenFrequencies = append(tokenFrequencies, TokenFrequency{Token: token, Count: count})
	}

	// Сортировка
	switch sortType {
	case "freq":
		log.Println("Sorting by frequency")
		sort.Slice(tokenFrequencies, func(i, j int) bool {
			return tokenFrequencies[i].Count > tokenFrequencies[j].Count
		})
	case "alpha":
		log.Println("Sorting alphabetically")
		sort.Slice(tokenFrequencies, func(i, j int) bool {
			return tokenFrequencies[i].Token < tokenFrequencies[j].Token
		})
	}

	// Записываем отсортированные данные в файл
	for _, tf := range tokenFrequencies {
		file.WriteString(fmt.Sprintf("%s %d\n", tf.Token, tf.Count))
	}

	return nil
}

// Обработка файлов и создание словаря
func (t *Tokenizer) ProcessFiles(dirPath string, maxGoroutines int, outputFile string, sortType string) error {
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
			var reader io.Reader

			// Открываем файл
			file, err := os.Open(filePath)
			if err != nil {
				t.logError(fmt.Sprintf("Error opening file %s: %v", filePath, err))
				t.copyErrorFile(filePath)
				return
			}
			defer file.Close()

			// Если файл в формате .gz, распаковываем его
			if strings.HasSuffix(fileEntry.Name(), ".gz") {
				gzReader, err := gzip.NewReader(file)
				if err != nil {
					t.logError(fmt.Sprintf("Error decompressing file %s: %v", filePath, err))
					t.copyErrorFile(filePath)
					return
				}
				defer gzReader.Close()
				reader = gzReader
			} else {
				reader = file
			}

			// Обработка файла
			localVocab := make(map[string]int)
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				tokens := segment.NewTokenizer().Tokenize(line)
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
				t.logError(fmt.Sprintf("Error reading file %s: %v", filePath, err))
				t.copyErrorFile(filePath)
				return
			}

			mutex.Lock()
			for token, count := range localVocab {
				vocab[token] += count
			}
			mutex.Unlock()

			progressMutex.Lock()
			processedFiles++
			fmt.Printf("\rProgress: %d/%d files processed (%.2f%%)", processedFiles, totalFiles, float64(processedFiles)/float64(totalFiles)*100)
			progressMutex.Unlock()
		}(fileEntry)
	}

	wg.Wait()
	fmt.Println()

	// Сохранение словаря
	return t.SaveVocabulary(vocab, outputFile, sortType)
}

// Логирование ошибок
func (t *Tokenizer) logError(message string) {
	log.New(t.logFile, "", log.LstdFlags).Println(message)
}

// Копирование проблемных файлов
func (t *Tokenizer) copyErrorFile(filePath string) {
	srcFile, err := os.Open(filePath)
	if err != nil {
		t.logError(fmt.Sprintf("Error opening error file %s: %v", filePath, err))
		return
	}
	defer srcFile.Close()

	dstPath := filepath.Join(t.errorDir, filepath.Base(filePath))
	dstFile, err := os.Create(dstPath)
	if err != nil {
		t.logError(fmt.Sprintf("Error creating error file copy %s: %v", dstPath, err))
		return
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		t.logError(fmt.Sprintf("Error copying error file %s: %v", filePath, err))
	}
}

// Проверка, является ли токен знаком препинания
func isPunctuation(token string) bool {
	for _, r := range token {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			return false
		}
	}
	return true
}
