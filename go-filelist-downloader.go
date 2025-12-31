// По умолчанию читается файл ./list, но можно опционально указать его параметром запуска.
// Также первым параметром можно задать -r, чтобы удалить исходный файл
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

//TODO Обработку параметров переписать на flag и добавить возможность ручного указания числа потоков

var settings = struct {
	sourceFileName       string // Имя входного файла
	targetFileNameLength int    // Какой длины имена файлов нужны на выходе
	parallelThreads      int
	deleteSourceFile     bool // Удалять ли исходный файл
}{
	sourceFileName:       "./list",
	targetFileNameLength: 3,
	parallelThreads:      3,
}

type chanStruct struct {
	fileLink    string
	fileCounter int
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == "-r" {
		settings.deleteSourceFile = true
	}

	if len(os.Args) > 2 || len(os.Args) == 2 && !settings.deleteSourceFile {
		settings.sourceFileName = os.Args[len(os.Args)-1]
	}
}

func main() {
	fData, err := os.ReadFile(settings.sourceFileName)
	if err != nil {
		log.Fatal("Ошибка при открытии входного файла", err)
		return
	}
	linksToDownload := strings.Split(string(fData), "\n")

	// Канал для передачи данных в горутиры
	queue := make(chan chanStruct, settings.parallelThreads)
	go func() {
		for counter, line := range linksToDownload {
			queue <- chanStruct{line, counter + 1}
		}
		close(queue)
	}()
	// Канал заполнен, можно читать его

	var wg sync.WaitGroup
	wg.Add(settings.parallelThreads)
	for v := settings.parallelThreads; v > 0; v-- {
		go getAndStore(queue, &wg)
	}

	fmt.Printf("Скачивается %d файлов\n", len(linksToDownload))
	wg.Wait()
	fmt.Println("Готово")

	// Удаление исходного файла, если это задано параметром запуска
	if settings.deleteSourceFile {
		os.Remove(settings.sourceFileName)
	}
}

// getAndStore читает файл согласно данным в канале и записывает в файл, имя которого задаётся числом. Расширение остаётся от оригинального прочитанного файла
func getAndStore(in <-chan chanStruct, wg *sync.WaitGroup) {
	defer wg.Done()

	for data := range in {
		urlParts, err := url.Parse(data.fileLink)
		if err != nil {
			log.Fatal("Плохой URL для разбора")
			return
		}

		fileContent, err := getLinkContent(data.fileLink)
		if err != nil || len(fileContent) == 0 {
			continue
		}

		fileName := getFileNameToStore(data.fileCounter, filepath.Ext(urlParts.Path))
		fmt.Printf("%s --> %s\n", data.fileLink, fileName)
		os.WriteFile(fileName, fileContent, 0644)
	}
}

// getLinkContent получает содержимое по зданной ссылке
func getLinkContent(link string) ([]byte, error) {
	response, err := http.Get(link)
	if err != nil {
		log.Fatal("Не удалось загрузить ", link)
		return nil, err
	}
	defer response.Body.Close()

	fileContent, err := io.ReadAll(response.Body)
	if err != nil {
		log.Fatal("Не получилось распознать ответ")
		return nil, err
	}

	return fileContent, nil
}

// getFileNameToStore возвращает имя файла, собранное из заданных частей и доведённое до нужной длины
func getFileNameToStore(number int, ext string) string {
	filename := strconv.Itoa(number)
	if len(filename) < settings.targetFileNameLength {
		zeroCount := settings.targetFileNameLength - len(filename)
		filename = strings.Repeat("0", zeroCount) + filename
	}

	return getFreeFilename(filename, ext)
}

// getFreeFilename получает незанятое имя файла, если переданное уже занятоь
func getFreeFilename(filename string, ext string) string {
	_, err := os.Stat(filename + ext)
	if err != nil {
		return filename + ext
	}

	return getFreeFilename(filename+"_", ext)
}
