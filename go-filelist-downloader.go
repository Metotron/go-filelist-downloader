package main

import (
	"bufio"
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

func main() {
	lines, fileName, err := readInputFileContent()
	if err != nil {
		log.Fatal("Ошибка при открытии входного файла", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(lines))

	counter := uint64(0)
	for _, line := range lines {
		counter++
		go getAndStore(line, counter, &wg)
	}

	wg.Wait()

	// Удаление исходного файла, если первым параметром указано -r
	if len(os.Args) > 1 && os.Args[1] == "-r" {
		os.Remove(fileName)
	}
}

func readInputFileContent() ([]string, string, error) {
	var fileName = "./list"
	if len(os.Args) > 2 || len(os.Args) == 2 && os.Args[1] != "-r" {
		fileName = os.Args[len(os.Args)-1]
	}

	fileData, err := os.Open(fileName)
	if err != nil {
		return nil, "", err
	}
	defer fileData.Close()

	lines := []string{}
	scanner := bufio.NewScanner(fileData)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines, fileName, nil
}

// Чтение данных по ссылке и запись в файл, имя которого задано числом. Расширение нужно оставить от оригинального файла
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
	}

	writeToFile(path, name, filepath.Ext(urlParts.Path), fileContent)
}

// Запись прочитанного содержимого в файл
func writeToFile(path string, name uint64, ext string, content []byte) {
	targetFileNameLength := 3 // Какой длины имена файлов нужны на выходе

	newFileName := strconv.FormatUint(name, 10)
	if len(newFileName) < targetFileNameLength {
		newFileName = strings.Repeat("0", targetFileNameLength-len(newFileName)) + newFileName
	}

	newFileName = newFileName + ext
	fmt.Printf("%s --> %s\n", path, newFileName)

	os.WriteFile(newFileName, content, 0644)
}
