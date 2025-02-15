package tokenizer

import (
	"bufio"
	"fmt"
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
}

func NewTokenizer(lowercase, filterPunct bool) *Tokenizer {
	return &Tokenizer{
		lowercase:   lowercase,
		filterPunct: filterPunct,
	}
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
			file, err := os.Open(filePath)
			if err != nil {
				fmt.Println("Error opening file:", err)
				return
			}
			defer file.Close()

			localVocab := make(map[string]int)
			scanner := bufio.NewScanner(file)
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
