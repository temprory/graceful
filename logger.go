package graceful

import (
	"github.com/temprory/log"
)

var (
	logDebug = log.Debug
	logInfo  = log.Info
	logWarn  = log.Warn
	logError = log.Error
	logPanic = log.Panic
	logFatal = log.Fatal
)

func SetDebugLogger(logger func(format string, v ...interface{})) {
	logDebug = logger
}

func SetInfoLogger(logger func(format string, v ...interface{})) {
	logInfo = logger
}

func SetWarnLogger(logger func(format string, v ...interface{})) {
	logWarn = logger
}

func SetErrorLogger(logger func(format string, v ...interface{})) {
	logError = logger
}

func SetPanicLogger(logger func(format string, v ...interface{})) {
	logPanic = logger
}

func SetFatalLogger(logger func(format string, v ...interface{})) {
	logFatal = logger
}
