package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type searchResult struct {
	content []resultEntry
	count   int
}

type resultEntry struct {
	path  string
	size  float64
	isDir bool
}

func (r *searchResult) add(path string, size int64, isDir bool) {
	fileSizeMB := float64(size) / 1024 / 1024
	if !isDir {
		r.count++
	}
	r.content = append(r.content, resultEntry{
		isDir: isDir,
		path:  path,
		size:  fileSizeMB,
	})
}

var playable map[string]bool = map[string]bool{}
var unplayable map[string]bool = map[string]bool{}

func fromFile(filename string) []string {
	result := []string{}
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		if len(txt) > 0 {
			result = append(result, txt)
		}
	}
	return result
}

func init() {
	for _, s := range fromFile("_playable") {
		playable[s] = true
	}
	for _, s := range fromFile("_unplayable") {
		unplayable[s] = true
	}
}

func getPartialKeyword(str string) string {
	_, rSize := utf8.DecodeLastRuneInString(str)

	if rSize <= 1 {
		return "" // not big char
	} else {
		if utf8.RuneCountInString(str) > 2 {
			return str[0 : len(str)-rSize]
		} else {
			return "" // only 1 or 2 big chars
		}
	}
}

func main() {
	var searchTerm string
	scanner := bufio.NewScanner(os.Stdin)
	if len(os.Args) == 2 {
		searchTerm = os.Args[1]
	} else {
		fmt.Print("Search term: ")
		if !scanner.Scan() {
			panic("Scanner did not return")
		}
		if scanner.Err() != nil {
			panic(scanner.Err())
		}
		searchTerm = scanner.Text()
	}
	results := &searchResult{}
	searchTermLower := strings.ToLower(searchTerm)
	partialKeyword := getPartialKeyword(searchTermLower)
	for drive := 'D'; drive <= 'Z'; drive++ {
		drivePath := fmt.Sprintf("%c:\\", drive)
		if _, err := os.Open(drivePath); err == nil {
			getDir(searchTermLower, partialKeyword, []string{drivePath}, results)
		}
	}
	fmt.Printf("%d files\n", results.count)
	for {
		fmt.Print("Keyword: (empty for all, or print all)\n")
		scanner.Scan()
		if scanner.Err() != nil {
			panic(scanner.Err())
		}
		keyword := scanner.Text()
		if keyword == "just print" {
			for _, r := range results.content {
				fmt.Println(r.path)
			}
			continue
		}
		if len(keyword) == 0 {
			keyword = searchTerm
		}
		filename := keyword
		os.Mkdir(".results", 0700)
		path := filepath.Join(".results", filename)
		m3u_path := filepath.Join(".results", filename+".m3u")
		// race condition: ctrl + c kills the program from another thread, sleep before opening file
		time.Sleep(100 * time.Millisecond)
		keywordLower := strings.ToLower(keyword)
		func() {
			f, err := os.Create(path)
			if err != nil {
				panic(err)
			}
			m3u_f, err := os.Create(m3u_path)
			if err != nil {
				panic(err)
			}
			defer func() {
				close_err := f.Close()
				if close_err != nil {
					fmt.Println(close_err)
				}
				close_err2 := m3u_f.Close()
				if close_err2 != nil {
					fmt.Println(close_err2)
				}
			}()
			for i, s := range results.content {
				path := s.path
				if isMatch("[NONE]", path, keywordLower, "") {
					if s.isDir {
						if i == len(results.content)-1 || !strings.HasPrefix(results.content[i+1].path, s.path) {
							printUtf16(f, "%s\n", path)
						}
					} else {
						printUtf16(f, "%s\t\t%f\n", path, s.size)
						dotIndex := strings.LastIndex(path, ".")
						if dotIndex >= 0 {
							ext := strings.ToLower(path[dotIndex+1:])
							if playable[ext] {
								printUtf16(m3u_f, "%s\n", path)
							} else if unplayable[ext] {

							} else {
								fmt.Printf("Unknow ext for: %s\n", path)
							}
						}
					}
				}
			}
		}()
		fmt.Printf("Fiile %s printed\n", path)
	}
}

func printUtf16(f *os.File, s string, args ...interface{}) {
	str := fmt.Sprintf(s, args...)
	f.WriteString(str)
}

func getDir(keyword string, partialKeyword string, dirs []string, results *searchResult) {
	dirName := filepath.Join(dirs...)
	files, _ := ioutil.ReadDir(dirName)
	for _, f := range files {
		if isMatch(dirName, f.Name(), keyword, partialKeyword) {
			target := filepath.Join(dirName, f.Name())
			if f.IsDir() {
				addAll(target, results)
			} else {
				results.add(target, f.Size(), false)
			}
		} else if f.IsDir() {
			getDir(keyword, partialKeyword, append(dirs, f.Name()), results)
		}
	}
}

func isMatch(dirName string, fname string, keyword string, partialKeyword string) bool {
	fnameLower := strings.ToLower(fname)
	if strings.Contains(fnameLower, keyword) {
		return true
	} else if partialKeyword != "" && strings.Contains(fnameLower, partialKeyword) {
		fmt.Printf("[WARN] partial match %s/%s\n", dirName, fname)
	}
	return false
}

func addAll(dir string, results *searchResult) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info == nil {
			panic("Nil info: \"" + path + "\"")
		}
		if info.IsDir() {
			results.add(path, 0, true)
		} else {
			results.add(path, info.Size(), false)
		}
		return nil
	})
}
