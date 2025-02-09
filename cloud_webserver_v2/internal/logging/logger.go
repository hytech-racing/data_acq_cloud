package logging

import (
	"fmt"
	"io"
	"log"
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
	writer   io.Writer
}

var instance *Logger
var once sync.Once
var originalStdout *os.File

// Intializes static logger (records last 10 logs by default)
func InitLogger() {
	once.Do(func() {
		instance = &Logger{
			logs:    make([]string, 10),
			maxLogs: 10,
		}
		instance.redirectStdoutToLogger()
	})
}

// Singleton method to make sure theres only one instance of logger
func GetLogger() *Logger {
	if instance == nil {
		panic("Logger not initialized. Call InitLogger() first.")
	}
	return instance
}

// Checks to see if the message is already formatted according to: "[YYYY-MM-DD HH:MM:SS]"
func isFormatted(msg string) bool {
	return len(msg) > 21 && msg[0] == '[' && msg[20] == ']'
}

// Stores the actual log in the array
func (l *Logger) storeLog(message string, level string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// If the message isn't formatter already, then format it according to our layout
	if !isFormatted(message) {
		message = fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)
	}

	l.logs[l.logIndex] = message
	l.logIndex = (l.logIndex + 1) % l.maxLogs
}

// Log something to terminal in the 2006-01-02 15:04:05 format
func (l *Logger) Log(level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	formattedMessage := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)
	fmt.Println(formattedMessage)

	l.storeLog(formattedMessage, level)
}

// LogF something to terminal in the 2006-01-02 15:04:05 format
func (l *Logger) LogF(level, format string, a ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	formattedMessage := fmt.Sprintf(format, a...);
	fmt.Printf("[%s] [%s] %s\n", timestamp, level, formattedMessage);

	l.storeLog(formattedMessage, level)
}

func (l * Logger) LogStdout(message string) {
	fmt.Println("This is redirected: ", message)
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
		index := (l.logIndex - 1 - i + l.maxLogs) % l.maxLogs

		if l.logs[index] != "" {
			recentLogs = append(recentLogs, l.logs[index])
		}
	}

	for i, j := 0, len(recentLogs)-1; i < j; i, j = i+1, j-1 {
		recentLogs[i], recentLogs[j] = recentLogs[j], recentLogs[i]
	}

	return recentLogs
}

// Redirect other stdout logs to the crash file
func (l *Logger) redirectStdoutToLogger() {
	originalStdout = os.Stdout

	reader, writer, err := os.Pipe()
	if err != nil {
		fmt.Printf("Failed to redirect stdout: %v\n", err)
		return
	}
	l.writer = writer

	// Since piping the stdout doesn't print it out into the terminal, we need to manually add it
	logWriter := io.MultiWriter(originalStdout, writer)
	log.SetFlags(0)
	log.SetOutput(logWriter) 
	log.SetPrefix(fmt.Sprintf("[%s] ", time.Now().Format("2006-01-02 15:04:05")))

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if err != nil {
				return
			}
			logMessage := string(buf[:n])
			l.storeLog(logMessage, "OLDLOGGER")
		}
	}()
}