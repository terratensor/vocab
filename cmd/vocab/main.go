package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof" // Импортируем pprof
	"os"
	"runtime"
	"time"

	"github.com/terratensor/vocab/internal/tokenizer"
)

func main() {
	// Определение флагов
	sortType := flag.String("sort", "", "Sort vocabulary by frequency (freq) or alphabetically (alpha)")
	lowercase := flag.Bool("lowercase", false, "Convert tokens to lowercase")
	filterPunct := flag.Bool("filter-punct", false, "Filter out punctuation tokens")
	dirPath := flag.String("dir", "./files", "Path to the directory containing text files")
	maxGoroutines := flag.Int("max-goroutines", 0, "Maximum number of goroutines (default: number of CPUs)")
	outputFile := flag.String("output", "vocab.txt", "Output file for the vocabulary")
	pprofFlag := flag.Bool("pprof", false, "Enable pprof profiling")
	flag.Parse()

	// Если maxGoroutines не указан, используем количество процессоров
	if *maxGoroutines <= 0 {
		*maxGoroutines = runtime.NumCPU()
		fmt.Printf("Using %d goroutines (number of CPUs)\n", *maxGoroutines)
	}

	// Включение pprof
	if *pprofFlag {
		go func() {
			fmt.Println("Starting pprof server on http://localhost:6060")
			if err := http.ListenAndServe("localhost:6060", nil); err != nil {
				fmt.Printf("Error starting pprof server: %v\n", err)
			}
		}()
		time.Sleep(1 * time.Second) // Даем время для запуска сервера
	}

	// Создание токенизатора
	tokenizer, err := tokenizer.NewTokenizer(*lowercase, *filterPunct)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	defer tokenizer.Close()

	// Обработка файлов
	err = tokenizer.ProcessFiles(*dirPath, *maxGoroutines, *outputFile, *sortType)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("Vocabulary saved to", *outputFile)
}
