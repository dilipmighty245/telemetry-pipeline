// Package logging provides a comprehensive logging system with dynamic level control and HTTP management.
//
// This package offers:
//   - Structured logging using Zap logger with configurable levels
//   - Global and file-specific log level control
//   - HTTP endpoint for runtime log level management
//   - Thread-safe operations with proper synchronization
//   - Support for debug, info, warn, error, and fatal log levels
//
// The logging system supports both global log levels and file-specific overrides,
// allowing fine-grained control over logging output in different parts of the application.
package logging

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// HTTPLogPort is the TCP port for the HTTP log server
	HTTPLogPort = 3000

	// HTTPLogPath is the HTTP path for the log server
	HTTPLogPath = "/log"
)

const (
	// Debug defines the debug logging level.
	Debug = iota
	// Info defines the info logging level.
	Info
	// Warn defines the warn logging level.
	Warn
	// Error defines the error logging level.
	Error
	// Fatal defines the Fatal logging level.
	Fatal
)

type payload struct {
	Level string `json:"level"`
}

type fileLogLevel struct {
	fileName string
	level    int
}

type logging struct {
	logger      *zap.Logger
	atom        zap.AtomicLevel
	globalLevel int
	fileLog     fileLogLevel
}

var logInstance *logging
var lock sync.RWMutex

// getFunctionName extracts the function name from the call stack.
//
// This function uses runtime.Caller to get the function name that called
// the logging function, providing context for log messages.
//
// Returns:
//   - string: The name of the calling function
func getFunctionName() string {
	pc, _, _, _ := runtime.Caller(3)
	function := runtime.FuncForPC(pc)
	FuncSplit := strings.Split(function.Name(), ".")
	return FuncSplit[len(FuncSplit)-1]
}

// getPackageName extracts the package name from the call stack.
//
// This function uses runtime.Caller to get the package name that contains
// the function calling the logging function.
//
// Returns:
//   - string: The name of the calling package
func getPackageName() string {
	pc, _, _, _ := runtime.Caller(3)
	functionFullName := runtime.FuncForPC(pc)
	function := filepath.Base(functionFullName.Name())
	FuncSplit := strings.Split(function, ".")
	return FuncSplit[0]
}

// getFileName extracts the filename from the call stack.
//
// This function uses runtime.Caller to get the filename that contains
// the function calling the logging function, used for file-specific log levels.
//
// Returns:
//   - string: The name of the calling file (without extension)
func getFileName() string {
	_, absFilename, _, _ := runtime.Caller(2)
	fileFullName := filepath.Base(absFilename)
	fileName := strings.TrimSuffix(fileFullName, filepath.Ext(fileFullName))
	return fileName
}

// logLevelToString converts a numeric log level to its string representation.
//
// Parameters:
//   - level: The numeric log level (Debug, Info, Warn, Error, Fatal)
//
// Returns:
//   - string: The string representation of the log level
func logLevelToString(level int) string {
	switch level {
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Error:
		return "error"
	case Fatal:
		return "fatal"
	}
	return "unknown"
}

// encodePayload encodes a payload as JSON and writes it to the HTTP response.
//
// Parameters:
//   - w: HTTP response writer
//   - data: The payload data to encode and send
func encodePayload(w http.ResponseWriter, data payload) {
	enc := json.NewEncoder(w)
	err := enc.Encode(data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode payload: %v", err), http.StatusInternalServerError)
	}
}

// ServeHTTP handles HTTP requests for log level management.
//
// Supported endpoints:
//   - GET /log: Get current global log level
//   - GET /log?file=<filename>: Get log level for specific file
//   - POST /log?level=<level>: Set global log level
//   - POST /log?file=<filename>&level=<level>: Set log level for specific file
//
// Parameters:
//   - w: HTTP response writer
//   - r: HTTP request with query parameters
//
// The function handles both global and file-specific log level queries and updates,
// returning appropriate JSON responses or HTTP error codes.
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	file := query.Get("file")
	level := query.Get("level")
	switch r.Method {
	case http.MethodGet:
		if file == "" {
			// GET http://localhost:HTTPLogPort/log
			// GET http://localhost:HTTPLogPort/log?file=
			encodePayload(w, payload{Level: logLevelToString(logInstance.globalLevel)})
		} else {
			// GET http://localhost:HTTPLogPort/log?file=<filename>
			if logInstance.fileLog.fileName != file {
				http.Error(w, "no log set for filename", http.StatusBadRequest)
				return
			}

			encodePayload(w, payload{Level: logLevelToString(logInstance.fileLog.level)})
		}
	case http.MethodPost:
		if level == "" {
			http.Error(w, "bad syntax", http.StatusBadRequest)
			return
		}
		// PUT http://localhost:HTTPLogPort/log?file=<filename>&level=<level>
		// PUT http://localhost:HTTPLogPort/log?level=<level>
		levelOk := SetLogLevel(level, file)
		if !levelOk {
			http.Error(w, "Invalid log level", http.StatusBadRequest)
			return
		}

		encodePayload(w, payload{Level: level})
	default:
		http.Error(w, "Only GET and POST are supported.", http.StatusBadRequest)
		return
	}

	_, err := w.Write([]byte{})
	if err != nil {
		Errorf("Failed to write response: %v", err)
	}
}

// init implements zap log initialization. Enables Debug logs based on:
//   - global log level
//   - file specific log level: FILE_NAME_DEBUG, where 'FILE_NAME' is the name of
//     the file the init is called from.
//     Example: logs from file 'graph.go' require an env variable: GRAPH_DEBUG
func init() {
	if logInstance != nil {
		return
	}

	logger, err := zap.NewProduction(zap.AddCallerSkip(1), zap.AddStacktrace(zap.FatalLevel))
	if err != nil {
		panic(err)
	}

	atom := zap.NewAtomicLevel()
	atom.SetLevel(zap.DebugLevel)
	logger = logger.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewCore(
			zapcore.NewConsoleEncoder(
				zap.NewDevelopmentEncoderConfig()),
			zapcore.AddSync(os.Stdout),
			atom,
		)
	}))

	// https://github.com/uber-go/zap/issues/880
	err = logger.Sync()
	if err != nil && reflect.TypeOf(err) != reflect.TypeOf(&fs.PathError{}) {
		panic(err)
	}

	logInstance = &logging{
		logger:      logger,
		atom:        atom,
		globalLevel: Debug,
	}

	if os.Getenv("ENABLE_LOGGING_ENDPOINT") == "true" {
		StartDefaultEndpoint()
	}
}

// StartDefaultEndpoint enables the logging controller endpoint on the default port and path.
//
// This function starts an HTTP server on the default port (3000) at the default path ("/log")
// for runtime log level management.
//
// Returns:
//   - *http.Server: The HTTP server instance for the logging endpoint
func StartDefaultEndpoint() *http.Server {
	return StartEndpoint(fmt.Sprintf(":%d", HTTPLogPort), "/log")
}

// StartEndpoint enables the logging controller endpoint on the specified address and path.
//
// This function starts an HTTP server for runtime log level management with custom
// address and path configuration.
//
// Parameters:
//   - addr: The address to bind the server to (e.g., ":3000")
//   - path: The HTTP path for the logging endpoint (e.g., "/log")
//
// Returns:
//   - *http.Server: The HTTP server instance for the logging endpoint
func StartEndpoint(addr, path string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(path, ServeHTTP)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			Errorf("logging server failed: %v", err)
		}
	}()

	return srv
}

// SetLogLevel sets the logging level for global or file-specific logging.
//
// This function allows dynamic adjustment of log levels at runtime, supporting
// both global level changes and file-specific overrides.
//
// Parameters:
//   - level: The log level string ("debug", "info", "warn", "error", "fatal")
//   - file: Optional filename for file-specific log level (empty string for global)
//
// Returns:
//   - bool: true if the log level was successfully set, false for invalid levels
func SetLogLevel(level string, file string) bool {
	level = strings.ToLower(level)
	var lvl int
	switch level {
	case "debug":
		lvl = Debug
	case "info":
		lvl = Info
	case "warn":
		lvl = Warn
	case "error":
		lvl = Error
	case "fatal":
		lvl = Fatal
	default:
		return false
	}
	if file != "" {
		lock.Lock()
		logInstance.fileLog.fileName = file
		logInstance.fileLog.level = lvl
		lock.Unlock()
		return true
	}

	lock.Lock()
	logInstance.globalLevel = lvl
	lock.Unlock()
	return true
}

// checkfileLogLevelMet checks if a file has a specific log level set and if the level threshold is met.
//
// Parameters:
//   - file: The filename to check
//   - level: The log level to check against
//
// Returns:
//   - bool: true if a log level is set for the input file
//   - bool: true if the log level threshold is met for the input file
func checkfileLogLevelMet(file string, level int) (bool, bool) {
	lock.RLock()
	defer lock.RUnlock()
	if logInstance.fileLog.fileName == file {
		if logInstance.fileLog.level <= level {
			return true, true
		}
		return true, false
	}
	return false, false
}

// checkGlobalLevelMet checks if the global log level threshold is met.
//
// Parameters:
//   - level: The log level to check against the global threshold
//
// Returns:
//   - bool: true if the global log level threshold is met
func checkGlobalLevelMet(level int) bool {
	lock.RLock()
	defer lock.RUnlock()
	return logInstance.globalLevel <= level
}

// logFormat formats a log message with package and function context.
//
// Parameters:
//   - msg: The log message format string
//   - args: Arguments for the format string
//
// Returns:
//   - string: Formatted log message with package and function context
func logFormat(msg string, args ...interface{}) string {
	s := fmt.Sprintf(msg, args...)
	m := fmt.Sprintf("%s  %s  %s", getPackageName(), getFunctionName(), s)
	return m
}

// checkLogLevel determines if logging should occur based on file-specific or global log levels.
//
// Parameters:
//   - file: The filename to check for file-specific log levels
//   - level: The log level to check against
//
// Returns:
//   - bool: true if logging should occur at the specified level
func checkLogLevel(file string, level int) bool {
	globalLvlMet := false
	fileFound, fileLvlMet := checkfileLogLevelMet(file, level)
	if !fileFound {
		globalLvlMet = checkGlobalLevelMet(level)
	}

	return fileLvlMet || globalLvlMet
}

// Debugf logs a message at Debug level with printf-style formatting.
//
// Parameters:
//   - msg: The log message format string
//   - args: Arguments for the format string
//
// The message is only logged if the debug level is enabled globally or for the calling file.
func Debugf(msg string, args ...interface{}) {
	if checkLogLevel(getFileName(), Debug) {
		s := logFormat(msg, args...)
		logInstance.logger.Debug(s)
	}
}

// Infof logs a message at Info level with printf-style formatting.
//
// Parameters:
//   - msg: The log message format string
//   - args: Arguments for the format string
//
// The message is only logged if the info level is enabled globally or for the calling file.
func Infof(msg string, args ...interface{}) {
	if checkLogLevel(getFileName(), Info) {
		s := logFormat(msg, args...)
		logInstance.logger.Info(s)
	}
}

// Warnf logs a message at Warn level with printf-style formatting.
//
// Parameters:
//   - msg: The log message format string
//   - args: Arguments for the format string
//
// The message is only logged if the warn level is enabled globally or for the calling file.
func Warnf(msg string, args ...interface{}) {
	if checkLogLevel(getFileName(), Warn) {
		s := logFormat(msg, args...)
		logInstance.logger.Warn(s)
	}
}

// Errorf logs a message at Error level with printf-style formatting.
//
// Parameters:
//   - msg: The log message format string
//   - args: Arguments for the format string
//
// The message is only logged if the error level is enabled globally or for the calling file.
func Errorf(msg string, args ...interface{}) {
	if checkLogLevel(getFileName(), Error) {
		s := logFormat(msg, args...)
		logInstance.logger.Error(s)
	}
}

// Fatalf logs a message at Fatal level with printf-style formatting and terminates the program.
//
// Parameters:
//   - msg: The log message format string
//   - args: Arguments for the format string
//
// This function always logs the message regardless of log level settings and then
// terminates the program execution.
func Fatalf(msg string, args ...interface{}) {
	s := logFormat(msg, args...)
	logInstance.logger.Fatal(s)
}
