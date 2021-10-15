package main

import (
	"encoding/json"
	"io/ioutil"
	"fmt"
	"os"

	"flag"
	"strings"

	"log"
	"strconv"
	"io"
	"time"
	"os/user"
	"path/filepath"

	"bufio"
	"sync"
	//"unicode"
	"errors"
)


/** logging **/
type logWriter struct {
    writer io.Writer
	timeFormat string
	user string
	hostname string
}

func (w logWriter) Write(b []byte) (n int, err error) {
	return w.writer.Write([]byte(time.Now().Format(w.timeFormat) + "\t" + "ptail@" + w.hostname + "\t" + w.user + "\t" + strconv.Itoa(os.Getpid()) + "\t" + string(b)))
}

/** tailer **/

type tailReader struct {
    io.ReadSeeker
    io.Closer
}

type Location struct {
	Offset int64
	Whence int
}

func (t tailReader) Read(b []byte) (int, error) {
	if t.ReadSeeker == nil {
		return 0, nil
	}
    for {
        n, err := t.ReadSeeker.Read(b)
        if n > 0 {
            return n, nil
        } else if err != io.EOF {
            return n, err
        }
        time.Sleep(10 * time.Millisecond)
    }
}

func newTailReader(fileName string, seek Location) (tailReader, error) {
	_,err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return tailReader{}, errors.New("file doesn't exist yet -" + string(fileName))
	}

    f, err := os.Open(fileName)
    if err != nil {
        return tailReader{}, err
    }

    if _, err := f.Seek(seek.Offset, seek.Whence); err != nil {
        return tailReader{}, err
    }
    return tailReader{f,f}, nil
}

func readerRoutine(fileName string, reader *tailReader) chan bool {
	ch := make(chan bool)
	go func() {

		scanner := bufio.NewScanner(*reader)
		for {
			select {
				case <-ch:
					return
				default:
					if scanner.Scan() {
						/*s := scanner.Text()
						upto := len(s)
						if upto > 10 {
							upto = 10
						}
						for i := 0; i < upto; i++ {
							if s[i] > unicode.MaxASCII {
								continue
							}
						}*/
                        // here we actually print out the lines..
                        if sourceFile {
                            fmt.Print(fileName + string('\t'))
                        }
                        fmt.Println(scanner.Text())
					}
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Println("err-reading:", err)
		}
		//fmt.Println("shutting down")
	}()
	return ch
}

/** argparse **/
type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ",")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var fileGlobs arrayFlags
var sourceFile bool


func getreader(file string, persistState map[string]int64, forceStart bool, logger *log.Logger) (*tailReader, error) {
	var reader tailReader
	var err error

	var seek Location
	file, err = filepath.Abs(file)
	if err != nil {
		logger.Println("could not figure out abs path of file " + file + ", skipping file")
		return &reader, err
	}

	if val, ok := persistState[file]; ok {
		seek.Offset = val
		seek.Whence = 0 // os.SEEK_SET
	} else {
		seek.Offset = 0
		seek.Whence = 2 // os.SEEK_END
	}

	_,err = os.Stat(file)
	if os.IsNotExist(err) || forceStart { // if file doesn't exist yet, then force to seek from start
		seek.Offset = 0
		seek.Whence = 0 //os.SEEK_SET
	}

	t, err := newTailReader(file, seek)

	if err != nil {
		return &reader, err
	} else {
		reader = t
	}
	return &reader, nil
}

// from https://stackoverflow.com/questions/35809252/check-if-flag-was-provided-in-go
// thanks to Markus Heukelom
func isFlagPassed(name string) bool {
    found := false
    flag.Visit(func(f *flag.Flag) {
        if f.Name == name {
            found = true
        }
    })
    return found
}

// start ** glob support
// this little gadget is from https://github.com/yargevad/filepathx/blob/master/filepathx.go
// thanks to Dave Gray

// Globs represents one filepath glob, with its elements joined by "**".
type Globs []string

// Glob adds double-star support to the core path/filepath Glob function.
// It's useful when your globs might have double-stars, but you're not sure.
func Glob(pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		// passthru to core package if no double-star
		return filepath.Glob(pattern)
	}
	return Globs(strings.Split(pattern, "**")).Expand()
}

// Expand finds matches for the provided Globs.
func (globs Globs) Expand() ([]string, error) {
	var matches = []string{""} // accumulate here
	for _, glob := range globs {
		var hits []string
		var hitMap = map[string]bool{}
		for _, match := range matches {
			paths, err := filepath.Glob(match + glob)
			if err != nil {
				return nil, err
			}
			for _, path := range paths {
				err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					// save deduped match from current iteration
					if _, ok := hitMap[path]; !ok {
						hits = append(hits, path)
						hitMap[path] = true
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			}
		}
		matches = hits
	}

	// fix up return value for nil input
	if globs == nil && len(matches) > 0 && matches[0] == "" {
		matches = matches[1:]
	}

	return matches, nil
}
// end ** glob support

/** additional helper functions **/

/**
	search globbed files
**/
func globber(input arrayFlags) []string {
	files := make([]string, 0)
	for _,file := range fileGlobs {
		globbed,_ := Glob(file)
		//fmt.Println(file)
		if len(globbed) > 0 {
			files = append(files, globbed...)
		} else {
			files = append(files, file)
		}
	}
	return files
}
func main() {
	flag.Var(&fileGlobs, "file", "file (or glob) to tail (can be used multiple times)")
	logToFile := flag.String("log", "", "enable logging. \"-\" for stdout, filename otherwise")
	persist := flag.Int64("persist", 0, "interval in milliseconds for persisting state (default is 0 - disabled)")
    sourceFileFlag := flag.Bool("sourcefile", false, "prefix source filename to every line (separated with tab)")
	persistFile := flag.String("statefile", "state.json", "statefile to be used for persistence")
	glob := flag.Int64("glob", 0, "interval in seconds for re-running glob search (default is 0 - disabled; only initially found files will be monitored). Will be auto-set to 1 if globbing detected.")
	wait := flag.Bool("wait", true, "wait for files to appear, don't exit program if monitored (and actually existing) filecount is 0")
	flag.Parse()

    sourceFile = *sourceFileFlag

	/**
		set up logging
	**/

	var logger *log.Logger
	if *logToFile == "" {
		logger = log.New(ioutil.Discard, "", 0)
	} else { // enable logging
		// get user info for log
		userobj, err := user.Current()
		usern := "-"
		if err != nil {
			log.Println("failed getting current user")
		}
		usern = userobj.Username

		// get host info for log
		hostname, err := os.Hostname()
		if err != nil {
			log.Println("failed getting machine hostname")
			hostname = "-"
		}

		if *logToFile == "-" { // to stdout
			logger = log.New(os.Stdout, "", 0)
			logger.SetFlags(0)
			logger.SetOutput(logWriter{writer: logger.Writer(), timeFormat: "2006-01-02T15:04:05.999Z07:00", user: usern, hostname: hostname})
		} else { // to file
			// get binary path
			logpath := *logToFile
			dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
			if err != nil {
				log.Println("couldn't get binary path - logfile path is relative to exec dir")
			} else {
				logpath = dir + "/" + *logToFile
			}

			f, err := os.OpenFile(logpath, os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
			if err != nil {
				log.Println("opening file " + *logToFile + " failed, writing log to stdout")
			} else {
				defer f.Close()
				logger = log.New(f, "", 0)
				logger.SetOutput(logWriter{writer: logger.Writer(), timeFormat: "2006-01-02T15:04:05.999Z", user: usern, hostname: hostname})
			}
		}
	}

	if len(fileGlobs) == 0 {
		logger.Println("no tail file specified, exiting")
		os.Exit(1)
	}


	globSet := false
	for _,file := range fileGlobs {
		if strings.Contains(file, "*") || strings.Contains(file, "?") || strings.Contains(file, "[") || strings.Contains(file, "]") {
			globSet = true
		}
	}
	
	if globSet && !isFlagPassed("glob") {
		*glob = 1
		logger.Println("detected globbing pattern, overriding -glob value to 1; set 0 excplicitly to disable")
	}

	/**
		read persistence data
	**/

	var persistState map[string]int64
	//var err error

	if *persist > 0 {
		jsonFile, err := os.Open(*persistFile)
		if err != nil {
			logger.Println("can't open persist file, continuing without persistence")
		} else {
			byteValue, _ := ioutil.ReadAll(jsonFile)
			json.Unmarshal(byteValue, &persistState)
		}
	}

	if persistState == nil {
		persistState = make(map[string]int64)
	}

	files := globber(fileGlobs)

	/*if !isFlagPassed("sourcefile") && (len(files) > 1 || globSet) {
		logger.Println("multiple files found or glob pattern detectd, auto-enabling source file output. explicitly set -sourcefile=false (with =) to disable")
		sourceFile = true
	}*/

	readers := make(map[string]*tailReader)
	channels := make(map[string]chan bool)

	var wg sync.WaitGroup

	logger.Println("starting with conf values - glob:", *glob, ", persist:", *persist, ", statefile:", *persistFile)

	for _,fileName := range files {
		reader, err := getreader(fileName, persistState, false, logger)
		if err == nil {
			readers[fileName] = reader
			channel := readerRoutine(fileName, reader)
			channels[fileName] = channel
		}
	}
	wg.Add(len(readers))

	if *wait {
		wg.Add(1)
	}
	keys := make([]string, 0, len(readers))
	for k := range readers {
		keys = append(keys, k)
	}
	logger.Println("initialized all readers:", keys)

	// start perodical globbing
	if *glob > 0 {
		go func() {
			for range time.Tick(time.Duration(*glob) * time.Second) {
				files = globber(fileGlobs)
				fileMap := make(map[string]int)
				//readers = getreaders(files, persistState, true, logger)
				for idx,fileName := range files {
					fileMap[fileName]=idx
					if _, ok := channels[fileName]; !ok {
						newReader, err := getreader(fileName, persistState, true, logger)
						if err == nil {
							logger.Println("new file", fileName)
							readers[fileName] = newReader
							channel := readerRoutine(fileName, newReader)
							channels[fileName] = channel
							wg.Add(1)
						}
					}
				}
				for fileName,_ := range channels {
					if _, ok := fileMap[fileName]; !ok {
						logger.Println("removing file", fileName)
						close(channels[fileName])
						delete(channels, fileName)
						readers[fileName].Close()
						delete(readers, fileName)
						wg.Done()
					}
				}
			}
		}()

	}

	// start periodical persistence
	if *persist > 0 {
		go func () {
			for range time.Tick(time.Duration(*persist) * time.Millisecond) {
				//var curPos int64
				//curPos = 0
				persistNewState := make(map[string]int64)
				for fileName, tailReader := range readers {
					curPos, err := tailReader.Seek(0, io.SeekCurrent)
					if err != nil {
						logger.Println("err - tell() error, could not get current pos in file", err)
					} else {
						persistNewState[fileName] = curPos
					}
				}

				file, err := json.Marshal(persistNewState)
				if err != nil {
					logger.Println("err - could not marshal persist state", err)
				}
				err = ioutil.WriteFile(*persistFile, file, 0644)
				if err != nil {
					logger.Println("err - could not persist state", err)
				}
			}
		}()
	}

	// wait for... santa?
	wg.Wait()

}
