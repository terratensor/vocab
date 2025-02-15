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

			// Построчное чтение и токенизация
			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1<<20) // 1 МБ
			scanner.Buffer(buf, 1<<20) // Устанавливаем максимальный размер токенаров
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
	return t.saveVocabulary(vocab, outputFile, sortType)
}

func (t *Tokenizer) saveVocabulary(vocab map[string]int, outputFile string, sortType string) error {
	file, err := os.Create(outputFile)
	if err != nil {
		t.logError(fmt.Sprintf("Error creating output file %s: %v", outputFile, err))
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

func (t *Tokenizer) logError(message string) {
	log.New(t.logFile, "", log.LstdFlags).Println(message)
}

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

func isPunctuation(token string) bool {
	for _, r := range token {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			return false
		}
	}
	return true
}
