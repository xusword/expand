package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

type matcher struct {
	keywords []string
	partials []string
}

func NewMatcher(searchTermInput string) *matcher {
	searchTermLower := strings.ToLower(searchTermInput)
	searchTermTokens := strings.Split(searchTermLower, "=")

	derivedTerms := []string{}
	for _, term := range searchTermTokens {
		spaceAsDot := strings.ReplaceAll(term, " ", ".")
		if spaceAsDot != term {
			derivedTerms = append(derivedTerms, spaceAsDot)
		}
	}

	partials := []string{}
	for _, term := range searchTermTokens {
		partial := getPartialKeyword(term)
		if partial != "" {
			partials = append(partials, partial)
		}
	}

	return &matcher{
		keywords: append(searchTermTokens, derivedTerms...),
		partials: partials,
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

func (m *matcher) isMatch(fName string, parentName string) bool {
	fnameLower := strings.ToLower(fName)
	for _, keyword := range m.keywords {
		if strings.Contains(fnameLower, keyword) {
			return true
		}
	}
	for _, partial := range m.partials {
		if strings.Contains(fnameLower, partial) {
			fmt.Printf("[WARN] partial match %s/%s\n", parentName, fName)
			// return false captured later
		}
	}
	return false
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

func controlledPanic(err error) {
	fmt.Printf("%+v", err)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	panic(err)
}

func fromFile(filename string) []string {
	result := []string{}
	f, err := os.Open(filename)
	if err != nil {
		controlledPanic(err)
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

func canonicalizeSearchTerms(term string) string {
	tokens := strings.Split(term, "=")
	sort.Strings(tokens)
	ordered := strings.Join(tokens, "=")
	return ordered
}

func main() {
	var searchTerm string
	scanner := bufio.NewScanner(os.Stdin)
	if len(os.Args) == 2 {
		searchTerm = os.Args[1]
	} else {
		fmt.Print("Search term: ")
		if !scanner.Scan() {
			controlledPanic(fmt.Errorf("scanner did not return"))
		}
		if scanner.Err() != nil {
			controlledPanic(scanner.Err())
		}
		searchTerm = scanner.Text()
	}
	results := &searchResult{}
	m := NewMatcher(searchTerm)
	for drive := 'D'; drive <= 'Z'; drive++ {
		drivePath := fmt.Sprintf("%c:\\", drive)
		if _, err := os.Open(drivePath); err == nil {
			getDir(m, []string{drivePath}, results)
		}
	}
	fmt.Printf("%d files\n", results.count)
	for {
		fmt.Print("Keyword: (empty for all, or print all)\n")
		scanner.Scan()
		if scanner.Err() != nil {
			controlledPanic(scanner.Err())
		}
		keyword := scanner.Text()
		if keyword == "just print" {
			for _, r := range results.content {
				fmt.Println(r.path)
			}
			continue
		}
		if len(keyword) == 0 {
			keyword = canonicalizeSearchTerms(searchTerm)
		}
		filename := keyword
		os.Mkdir(".results", 0700)
		path := filepath.Join(".results", filename)
		m3u_path := filepath.Join(".results", filename+".m3u")
		// race condition: ctrl + c kills the program from another thread, sleep before opening file
		time.Sleep(100 * time.Millisecond)
		subquery := NewMatcher(keyword)
		func() {
			// put this in a function to force defer close to happen
			f, err := os.Create(path)
			if err != nil {
				controlledPanic(err)
			}
			m3u_f, err := os.Create(m3u_path)
			if err != nil {
				controlledPanic(err)
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
				if subquery.isMatch(path, "[NOTE: no partent is given in subqueries]") {
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

func getDir(m *matcher, dirs []string, results *searchResult) {
	dirName := filepath.Join(dirs...)
	files, _ := os.ReadDir(dirName)
	for _, f := range files {
		if m.isMatch(f.Name(), dirName) {
			target := filepath.Join(dirName, f.Name())
			if f.IsDir() {
				addAll(target, results)
			} else {
				fInfo, fErr := f.Info()
				if fErr != nil {
					fmt.Printf("[Error] Error getting file info %f", fErr)
				}
				results.add(target, fInfo.Size(), false)
			}
		} else if f.IsDir() {
			getDir(m, append(dirs, f.Name()), results)
		}
	}
}

func addAll(dir string, results *searchResult) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info == nil {
			controlledPanic(fmt.Errorf("Nil info: \"" + path + "\""))
		}
		if info.IsDir() {
			results.add(path, 0, true)
		} else {
			results.add(path, info.Size(), false)
		}
		return nil
	})
}
