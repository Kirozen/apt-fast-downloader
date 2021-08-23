package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

type DownloadFile struct {
	Urls        []string
	Destination string
	Filename    string
}

func NewDownloadFile(urls []string, destination string) DownloadFile {
	return DownloadFile{urls, destination, filepath.Base(urls[0])}
}

func (df DownloadFile) Filepath() string {
	return filepath.Join(df.Destination, df.Filename)
}

const (
	BUFFER_SIZE = 32768
)

func ParseAria2(data []string, destination string) (files []DownloadFile) {
	outRe, err := regexp.Compile(`^\s+out=(?P<out>.*)$`)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, line := range data {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "http") {
			files = append(files, NewDownloadFile(strings.Split(line, "\t"), destination))
		} else {
			if outRe.MatchString(line) {
				index := outRe.SubexpIndex("out")
				files[len(files)-1].Filename = outRe.FindStringSubmatch(line)[index]
			}
		}
	}
	return
}

func GetInput(input string, destination string, aria2Compatibility bool) (paths []DownloadFile) {
	lines := ReadLines(input)
	if aria2Compatibility {
		paths = append(paths, ParseAria2(lines, destination)...)
	} else {
		for _, url := range lines {
			if strings.HasPrefix(url, "http") {
				urls := []string{url}
				paths = append(paths, NewDownloadFile(urls, destination))
			}
		}
	}
	return
}

func ReadLines(filename string) (lines []string) {
	f, err := os.Open(filename)

	if err != nil {
		fmt.Println(err)
		return
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		fmt.Println(err)
	}
	return
}

func Downloader(downloadFile DownloadFile, bufferSize int, quiet bool) {
	// Create blank file
	file, err := os.Create(downloadFile.Filepath())
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	// Put content on file
	resp, err := client.Get(downloadFile.Urls[0])
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	size, err := io.Copy(file, resp.Body)

	fmt.Printf("Downloaded a file %s with size %d", downloadFile.Filename, size)

}

func main() {
	var threads = flag.Int("t", 0, "thread number")
	var quiet = flag.Bool("q", false, "quiet")
	var destination = flag.String("d", "", "destination")
	var aria2Compatibility = flag.Bool("a", false, "aria2 compatibility")
	var bufferSize = flag.Int("b", BUFFER_SIZE, "buffer size")
	var inputFile = flag.String("i", "", "input file")
	flag.Parse()

	remainingArgs := len(os.Args) - flag.NArg() - 1
	urls := os.Args[remainingArgs:]

	var downloadUrls []DownloadFile

	for _, url := range urls {
		downloadUrls = append(downloadUrls, NewDownloadFile([]string{url}, *destination))
	}

	downloadUrls = append(downloadUrls, GetInput(*inputFile, *destination, *aria2Compatibility)...)

	var wg sync.WaitGroup
	// passed &wg will be accounted at p.Wait() call
	p := mpb.New(mpb.WithWaitGroup(&wg))
	total, numBars := 100, 3
	wg.Add(numBars)

	for i := 0; i < numBars; i++ {
		name := fmt.Sprintf("Bar#%d:", i)
		bar := p.AddBar(int64(total),
			mpb.PrependDecorators(
				// simple name decorator
				decor.Name(name),
				// decor.DSyncWidth bit enables column width synchronization
				decor.Percentage(decor.WCSyncSpace),
			),
			mpb.AppendDecorators(
				// replace ETA decorator with "done" message, OnComplete event
				decor.OnComplete(
					// ETA decorator with ewma age of 60
					decor.EwmaETA(decor.ET_STYLE_GO, 60, decor.WCSyncWidth), "done",
				),
			),
		)
		// simulating some work
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			max := 100 * time.Millisecond
			for i := 0; i < total; i++ {
				// start variable is solely for EWMA calculation
				// EWMA's unit of measure is an iteration's duration
				start := time.Now()
				time.Sleep(time.Duration(rng.Intn(10)+1) * max / 10)
				bar.Increment()
				// we need to call DecoratorEwmaUpdate to fulfill ewma decorator's contract
				bar.DecoratorEwmaUpdate(time.Since(start))
			}
		}()
	}
	// Waiting for passed &wg and for all bars to complete and flush
	p.Wait()
}
