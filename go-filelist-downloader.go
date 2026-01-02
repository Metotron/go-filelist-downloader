// По умолчанию читается файл ./list, но можно опционально указать его параметром запуска.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var settings struct {
	sourceFileName       string // Имя входного файла
	targetFileNameLength int    // Какой длины имена файлов нужны на выходе
	parallelThreads      int    // Сколько одновременных горутин запускать для скачивания
	deleteSourceFile     bool   // Удалять ли исходный файл
}

type chanStruct struct {
	fileLink    string
	fileCounter int
}

func init() {
	flag.BoolVar(&settings.deleteSourceFile, "r", false, "Удалить файл со ссылками после загрузки")
	flag.IntVar(&settings.parallelThreads, "t", 3, "Количество потоков для скачивания")
	flag.IntVar(&settings.targetFileNameLength, "l", 3, "Количество символов в имени конечного файла")

	flag.Parse()

	settings.sourceFileName = "./list"
	if len(flag.Args()) > 0 {
		settings.sourceFileName = flag.Arg(0)
	}
}

var downloadedCounter atomic.Int32 // Сколько файлов успешно скачано

var successCounter struct {
	counter int
	mutex   sync.Mutex
}

var linkToLocalName map[string]string = make(map[string]string) // Под какими именами сохранены запрошенные файлы (если сохранены)

func main() {
	_, err := os.Stat(settings.sourceFileName)
	if err != nil {
		fmt.Println("Файл не найден:", settings.sourceFileName)
		return
	}

	fData, err := os.ReadFile(settings.sourceFileName)
	if err != nil {
		fmt.Println("Ошибка при открытии входного файла\n", err)
		return
	}

	linksToDownload := strings.Split(string(fData), "\n")
	nonEmptyElementsCount := len(linksToDownload)

	for _, line := range linksToDownload {
		if len(line) == 0 {
			nonEmptyElementsCount--
		}
	}

	// Канал для передачи данных в горутины
	queue := make(chan chanStruct, settings.parallelThreads)
	go func() {
		for idx, line := range linksToDownload {
			if len(line) == 0 {
				continue
			}
			queue <- chanStruct{line, idx + 1}
		}
		close(queue)
	}()
	// Канал заполнен, можно читать его

	var wg sync.WaitGroup
	wg.Add(settings.parallelThreads)
	for v := settings.parallelThreads; v > 0; v-- {
		go getAndStore(queue, &wg)
	}

	fmt.Printf("Скачивается \033[33m%d\033[0m %s\n", nonEmptyElementsCount, wordForCount(nonEmptyElementsCount))
	wg.Wait()

	renameTmpFiles(linksToDownload)

	fmt.Printf("Готово: \033[33m%d/%d\033[0m\n", downloadedCounter.Load(), nonEmptyElementsCount)

	// Удаление файла со списком, если это задано параметром запуска
	if settings.deleteSourceFile {
		if err := os.Remove(settings.sourceFileName); err != nil {
			log.Fatal("Не получилось удалить файл списка " + settings.sourceFileName)
		}
	}
}

// getAndStore обращается по ссылке согласно данным в канале in и записывает результат в файл, имя которого задаётся числом.
// Расширение остаётся от оригинального прочитанного файла
func getAndStore(in <-chan chanStruct, wg *sync.WaitGroup) {
	defer wg.Done()

	for data := range in {
		_, err := url.Parse(data.fileLink)
		if err != nil {
			fmt.Println("\t\033[31mПлохой URL для разбора:", data.fileLink, "\033[0m")
			return
		}

		fileContent, err := getLinkContent(data.fileLink)
		if err != nil {
			fmt.Println(err)
			continue
		}

		if len(fileContent) == 0 {
			fmt.Println("\t\033[31mСервер прислал пустой ответ:", data.fileLink, "\033[0m")
			continue
		}

		downloadedCounter.Add(1)
		file, err := os.CreateTemp(".", "gget_*")
		if err != nil {
			log.Fatal("Не удаётся создать временный файл")
			return
		}
		defer file.Close()

		successCounter.mutex.Lock()
		successCounter.counter++
		fmt.Printf("[\033[96m%3d\033[0m]\033[92m %s\033[0m\n", successCounter.counter, linkCutter(data.fileLink, 76))
		successCounter.mutex.Unlock()

		if _, err := file.Write(fileContent); err != nil {
			fmt.Println("\t\033[31mНе удалось записать данные во временный файл ["+file.Name()+"]:", data.fileLink, "\033[0m")
			continue
		}
		file.Chmod(0o644)
		linkToLocalName[data.fileLink] = file.Name()
	}
}

// getLinkContent получает содержимое по зданной ссылке
func getLinkContent(link string) ([]byte, error) {
	response, err := http.Get(link)
	if err != nil {
		return nil, fmt.Errorf("\t\033[31mНе удалось загрузить %s.\033[0m%w", link, err)
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("\t\033[31mОшибка %d при запросе файла %s.\033[0m", response.StatusCode, link)
	}

	fileContent, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("\t\033[31mНе получилось распознать ответ от %s.\033[0m%w", link, err)
	}

	return fileContent, nil
}

// wordForCount формирует правильное окончание дляя слова "файл"
func wordForCount(n int) string {
	n100 := n % 100
	n10 := n % 10

	if n100 >= 5 && n100 < 20 || n10 == 0 {
		return "файлов"
	}

	if n10 == 1 {
		return "файл"
	}

	return "файла"
}

// linkCutter воззвращает ссылку урезанную до длины cutTo. Если длина обрезана, добавляется многоточие
func linkCutter(link string, cutToLength int) string {
	if len(link) > cutToLength {
		link = link[:cutToLength] + "…"
	}

	return link
}

// renameTmpFiles переименовывает временные файлы в числовые названия
func renameTmpFiles(list []string) {
	counter := 0 // имя файла станет числом на основании этого счётчика

	for _, filename := range list {
		tmpFileName, ok := linkToLocalName[filename]
		if !ok {
			continue // файл не был скачан — нечего обрабатывать
		}

		counter++
		newName := strconv.Itoa(counter)
		if len(newName) < settings.targetFileNameLength {
			zerosCount := settings.targetFileNameLength - len(newName)
			newName = strings.Repeat("0", zerosCount) + newName
		}

		nameCleaner := regexp.MustCompile(`[\?#].+$`)
		nameCleaner.Longest()
		clearFilename := nameCleaner.ReplaceAllString(filename, "")
		renameTo := findUnusedFilenameVariant(newName + filepath.Ext(clearFilename))
		os.Rename(tmpFileName, renameTo)
	}
}

func findUnusedFilenameVariant(filename string) string {
	if _, err := os.Stat(filename); err != nil {
		return filename
	}

	return findUnusedFilenameVariant(strings.TrimSuffix(filename, filepath.Ext(filename)) + "_" + filepath.Ext(filename))
}
