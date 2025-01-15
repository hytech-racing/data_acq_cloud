package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Logger struct {
	mu       sync.Mutex
	logs     []string
	logIndex int
	maxLogs  int
}

var instance *Logger
var once sync.Once

// Intializes static logger (records last 10 logs)
func InitLogger() {
	once.Do(func() {
		instance = &Logger{
			logs:    make([]string, 10),
			maxLogs: 10,
		}
	})
}

// Singleton method to make sure theres only one instance of logger
func GetLogger() *Logger {
	if instance == nil {
		panic("Logger not initialized. Call InitLogger() first.")
	}
	return instance
}

// Log something to terminal in the 2006-01-02 15:04:05 format
func (l *Logger) Log(level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	formattedMessage := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)
	fmt.Println(formattedMessage)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs[l.logIndex] = formattedMessage
	l.logIndex = (l.logIndex + 1) % l.maxLogs
}

func (l *Logger) Info(message string) {
	l.Log("INFO", message)
}

func (l *Logger) Debug(message string) {
	l.Log("DEBUG", message)
}

func (l *Logger) Warn(message string) {
	l.Log("WARN", message)
}

func (l *Logger) Error(message string) {
	l.Log("ERROR", message)
}

// Global recovery system
func (l *Logger) RecoverAndLogPanic() {
	if r := recover(); r != nil {
		l.WriteCrashFile(r);
	}
}

// Write all logs to file from the array
func (l *Logger) WriteCrashFile(r any) {
	recentLogs := l.GetRecentLogs()

	logDir := "/app/logs/crash/"
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		fmt.Printf("Failed to create log directory: %v\n", err)
		return
	}

	timestamp := time.Now().Format("20060102-150405")
	crashFile := filepath.Join(logDir, fmt.Sprintf("crash-%s.log", timestamp))
	file, err := os.Create(crashFile)
	if err != nil {
		fmt.Printf("Failed to create crash file: %v\n", err)
		return
	}
	defer file.Close()

	file.WriteString("==== Crash Report ====\n")
	file.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	file.WriteString(fmt.Sprintf("Panic: %v\n\n", r))
	file.WriteString("==== Last 10 Logs ====\n")
	for _, log := range recentLogs {
		file.WriteString(log + "\n")
	}
}

// Get the recent logs stored in the array
func (l *Logger) GetRecentLogs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	var recentLogs []string
	for i := 0; i < l.maxLogs; i++ {
		index := (l.logIndex + i) % l.maxLogs
		if l.logs[index] != "" {
			recentLogs = append(recentLogs, l.logs[index])
		}
	}
	return recentLogs
}