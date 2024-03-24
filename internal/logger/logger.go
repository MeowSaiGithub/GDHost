package logger

import (
	"github.com/rs/zerolog"
	logs "github.com/rs/zerolog/log"
	"os"
	"strings"
	"time"
)

func init() {
	zerolog.TimestampFieldName = "timestamp"
	zerolog.TimeFieldFormat = time.DateTime
}

// InitLog initializes log to use log level provided by config. Returns(logger *zerolog.Logger)
func InitLog(level string) *zerolog.Logger {
	parsedLogLevel := parseLogLevel(level)
	zerolog.SetGlobalLevel(parsedLogLevel)
	logger := logs.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.DateTime}).With().Timestamp().Logger()
	logger.Info().Msgf("current log level: %s", parsedLogLevel.String())
	return &logger
}

// parseLogLevel changes log level from the config to lower case and then to Level type. Returns (zerolog.Level)
func parseLogLevel(level string) zerolog.Level {
	level = strings.ToLower(level)
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}
