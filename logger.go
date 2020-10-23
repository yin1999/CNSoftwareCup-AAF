package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"
)

const (
	logPathFormat = "%s/%d-%d-%d.log"
)

// MultiLogger 用于创建os.Stdout, os.File 的logger
type MultiLogger struct {
	*log.Logger
	done chan struct{}
}

// NewMultiLogger 创建新的MuntiLogger
func NewMultiLogger(recordingTime time.Duration, dir string) *MultiLogger {
	if len(dir) == 0 {
		dir = "."
	}
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now()
	path := fmt.Sprintf(logPathFormat, dir, now.Year(), now.Month(), now.Day())
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	runtime.SetFinalizer(file, (*os.File).Close)
	logger := &MultiLogger{
		Logger: newLogger(os.Stdout, file),
		done:   make(chan struct{}),
	}
	go logger.logMaintainer(recordingTime, dir)
	return logger
}

func (logger *MultiLogger) logMaintainer(recordingTime time.Duration, dir string) {
	ticker := time.NewTicker(recordingTime)
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			path := fmt.Sprintf(logPathFormat, dir, now.Year(), now.Month(), now.Day())
			err := os.MkdirAll(dir, 0755)
			if err != nil {
				logger.Println(err)
				continue
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				logger.Println(err)
				continue
			}
			runtime.SetFinalizer(file, (*os.File).Close)
			logger.Logger = newLogger(file, os.Stdout)
		case <-logger.done:
			return
		}
	}
}

func newLogger(writers ...io.Writer) *log.Logger {
	writer := io.MultiWriter(writers...)
	return log.New(writer, "", log.Ldate|log.Ltime|log.Lshortfile)
}

// Done 关闭logger
func (logger *MultiLogger) Done() {
	logger.done <- struct{}{}
	logger.Logger = nil
}
