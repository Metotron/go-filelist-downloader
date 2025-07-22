// По умолчанию читается файл ./list, но можно опционально указать его параметром запуска. Также первым параметром можно задать -r, чтобы удалить исходный файл

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

type programSettings struct {
	sourceFileName       string // Имя входного файла
	targetFileNameLength int    // Какой длины имена файлов нужны на выходе
	deleteSourceFile     bool   // Удалять ли исходный файл
}

var settings = programSettings{
	sourceFileName:       "./list",
	targetFileNameLength: 3,
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
	lines := strings.Split(string(fData), "\n")

	var wg sync.WaitGroup
	wg.Add(len(lines))

	counter := uint64(0)
	for _, line := range lines { //TODO Поставить ограничение на число горутин и отдавать им строки через канал
		counter++
		go getAndStore(line, counter, &wg)
	}

	wg.Wait()

	// Удаление исходного файла, если это задано параметром запуска
	if settings.deleteSourceFile {
		os.Remove(settings.sourceFileName)
	}
}

// getAndStore читает данные по http-ссылке и записывает в файл, имя которого задано числом. Расширение остаётся от оригинального прочитанного файла
func getAndStore(path string, name uint64, wg *sync.WaitGroup) {
	defer wg.Done()

	urlParts, err := url.Parse(path)
	if err != nil {
		log.Fatal("Плохой URL для разбора")
		return
	}

	response, err := http.Get(path)
	if err != nil {
		log.Fatal("Не удалось загрузить ", path)
		return
	}
	defer response.Body.Close()

	fileContent, err := io.ReadAll(response.Body)
	if err != nil {
		log.Fatal("Не получилось распознать ответ")
		return
	}

	fileName := getFileNameToStore(name, filepath.Ext(urlParts.Path))
	os.WriteFile(fileName, fileContent, 0644)
	fmt.Printf("%s --> %s\n", path, fileName)
}

// getFileNameToStore возвращает имя файла, собранное из заданных частей и доведённое до нужной длины
func getFileNameToStore(name uint64, ext string) string {
	fileName := strconv.FormatUint(name, 10)
	if len(fileName) < settings.targetFileNameLength {
		zeroCount := settings.targetFileNameLength - len(fileName)
		fileName = strings.Repeat("0", zeroCount) + fileName
	}

	return fileName + ext
}
