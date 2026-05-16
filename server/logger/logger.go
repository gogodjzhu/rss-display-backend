package logger

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu      sync.Mutex
	logDir  string
	loggers = map[string]*log.Logger{}
)

func Init(dir string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	logDir = dir
	return nil
}

func Get(module string) *log.Logger {
	mu.Lock()
	defer mu.Unlock()

	if l, ok := loggers[module]; ok {
		return l
	}

	if logDir == "" {
		l := log.New(os.Stderr, "["+module+"] ", log.LstdFlags|log.Lmsgprefix)
		loggers[module] = l
		return l
	}

	path := filepath.Join(logDir, module+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[logger] failed to open log file %s: %v, falling back to stderr", path, err)
		l := log.New(os.Stderr, "["+module+"] ", log.LstdFlags|log.Lmsgprefix)
		loggers[module] = l
		return l
	}

	l := log.New(f, "["+module+"] ", log.LstdFlags|log.Lmsgprefix)
	loggers[module] = l
	return l
}

func MultiWriter(module string) io.Writer {
	mu.Lock()
	defer mu.Unlock()

	writers := []io.Writer{os.Stderr}

	if logDir != "" {
		path := filepath.Join(logDir, module+".log")
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			writers = append(writers, f)
		}
	}

	return io.MultiWriter(writers...)
}