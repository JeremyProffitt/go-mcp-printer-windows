package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message.
type LogLevel int

const (
	LevelOff LogLevel = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelAccess
	LevelDebug
)

func (l LogLevel) String() string {
	switch l {
	case LevelOff:
		return "OFF"
	case LevelError:
		return "ERROR"
	case LevelWarn:
		return "WARN"
	case LevelInfo:
		return "INFO"
	case LevelAccess:
		return "ACCESS"
	case LevelDebug:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}

func ParseLogLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "off":
		return LevelOff
	case "error":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "info":
		return LevelInfo
	case "access":
		return LevelAccess
	case "debug":
		return LevelDebug
	default:
		return LevelInfo
	}
}

type Logger struct {
	mu        sync.Mutex
	level     LogLevel
	logger    *log.Logger
	file      *os.File
	logDir    string
	appName   string
	startTime time.Time
}

type Config struct {
	LogDir  string
	AppName string
	Level   LogLevel
}

var (
	defaultLogger *Logger
	once          sync.Once
)

const DefaultAppName = "go-mcp-printer-windows"

func DefaultLogDir() string {
	return filepath.Join(os.Getenv("ProgramData"), DefaultAppName, "logs")
}

func Init(cfg Config) error {
	var initErr error
	once.Do(func() {
		defaultLogger, initErr = NewLogger(cfg)
	})
	return initErr
}

func NewLogger(cfg Config) (*Logger, error) {
	if cfg.AppName == "" {
		cfg.AppName = DefaultAppName
	}

	logDir := cfg.LogDir
	if logDir == "" {
		logDir = DefaultLogDir()
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	timestamp := time.Now().Format("2006-01-02")
	logFileName := fmt.Sprintf("%s-%s.log", cfg.AppName, timestamp)
	logPath := filepath.Join(logDir, logFileName)

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}

	l := &Logger{
		level:     cfg.Level,
		logger:    log.New(file, "", 0),
		file:      file,
		logDir:    logDir,
		appName:   cfg.AppName,
		startTime: time.Now(),
	}

	return l, nil
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if l == nil || level > l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	message := fmt.Sprintf(format, args...)
	l.logger.Printf("[%s] [%s] %s", timestamp, level.String(), message)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

func (l *Logger) Access(format string, args ...interface{}) {
	l.log(LevelAccess, format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.SetOutput(w)
}

// ToolCall logs a tool invocation with argument keys only (no values for safety).
func (l *Logger) ToolCall(toolName string, args map[string]interface{}, duration time.Duration, success bool) {
	argKeys := make([]string, 0, len(args))
	for k := range args {
		argKeys = append(argKeys, k)
	}
	l.Info("TOOL_CALL tool=%q args=%v duration=%s success=%v", toolName, argKeys, duration, success)
}

// PrintJob logs a print job submission.
func (l *Logger) PrintJob(printer, file string, copies int, err error) {
	if err != nil {
		l.Info("PRINT_JOB printer=%q file=%q copies=%d error=%q", printer, SanitizePath(file), copies, err.Error())
	} else {
		l.Info("PRINT_JOB printer=%q file=%q copies=%d", printer, SanitizePath(file), copies)
	}
}

// HTTPAccess logs an HTTP request.
func (l *Logger) HTTPAccess(method, path string, statusCode int, duration time.Duration) {
	l.Access("HTTP %s %s status=%d duration=%s", method, path, statusCode, duration)
}

// StartupInfo contains server startup details.
type StartupInfo struct {
	Version   string
	GoVersion string
	OS        string
	Arch      string
	NumCPU    int
	LogDir    string
	LogLevel  string
	Domain    string
	Port      int
	PID       int
	StartTime time.Time
}

func (l *Logger) LogStartup(info StartupInfo) {
	l.Info("========================================")
	l.Info("SERVER STARTUP")
	l.Info("========================================")
	l.Info("Application: %s", l.appName)
	l.Info("Version: %s", info.Version)
	l.Info("Go Version: %s", info.GoVersion)
	l.Info("OS: %s/%s", info.OS, info.Arch)
	l.Info("CPUs: %d", info.NumCPU)
	l.Info("PID: %d", info.PID)
	l.Info("Start Time: %s", info.StartTime.Format(time.RFC3339))
	l.Info("Log Directory: %s", info.LogDir)
	l.Info("Log Level: %s", info.LogLevel)
	l.Info("Domain: %s", info.Domain)
	l.Info("Port: %d", info.Port)
	l.Info("========================================")
}

func (l *Logger) LogShutdown(reason string) {
	uptime := time.Since(l.startTime)
	l.Info("========================================")
	l.Info("SERVER SHUTDOWN")
	l.Info("Reason: %s", reason)
	l.Info("Uptime: %s", uptime)
	l.Info("========================================")
}

func GetStartupInfo(version, logDir, logLevel, domain string, port int) StartupInfo {
	return StartupInfo{
		Version:   version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		LogDir:    logDir,
		LogLevel:  logLevel,
		Domain:    domain,
		Port:      port,
		PID:       os.Getpid(),
		StartTime: time.Now(),
	}
}

func GetLogger() *Logger {
	return defaultLogger
}

// --- Global convenience functions ---

func Error(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(format, args...)
	}
}

func Warn(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(format, args...)
	}
}

func Info(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(format, args...)
	}
}

func Access(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Access(format, args...)
	}
}

func Debug(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(format, args...)
	}
}

func ToolCall(toolName string, args map[string]interface{}, duration time.Duration, success bool) {
	if defaultLogger != nil {
		defaultLogger.ToolCall(toolName, args, duration, success)
	}
}

// --- PII filtering ---

var (
	ssnPattern           = regexp.MustCompile(`\b(\d{3}[-\s]?\d{2}[-\s]?\d{4})\b`)
	panPattern           = regexp.MustCompile(`\b(\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{1,7})\b`)
	panContinuousPattern = regexp.MustCompile(`\b(\d{13,19})\b`)
	jsonTokenPattern     = regexp.MustCompile(`("(?:access_token|token|api_key|apikey|secret|password|credential|bearer)":\s*")([^"]+)(")`)
)

// MaskSecret masks a secret value showing only the last 4 characters.
func MaskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "xxx" + secret
	}
	return "xxx" + secret[len(secret)-4:]
}

// SanitizePII removes or masks PII data from log messages.
func SanitizePII(message string) string {
	message = ssnPattern.ReplaceAllString(message, "[SSN-REDACTED]")
	message = panPattern.ReplaceAllString(message, "[PAN-REDACTED]")
	message = panContinuousPattern.ReplaceAllString(message, "[PAN-REDACTED]")
	message = jsonTokenPattern.ReplaceAllStringFunc(message, func(match string) string {
		parts := jsonTokenPattern.FindStringSubmatch(match)
		if len(parts) == 4 {
			return parts[1] + MaskSecret(parts[2]) + parts[3]
		}
		return match
	})
	return message
}

// SanitizePath removes potentially sensitive path components.
func SanitizePath(path string) string {
	// Only show filename, not full path which may reveal user directories
	return filepath.Base(path)
}
