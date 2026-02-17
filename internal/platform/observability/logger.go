package observability

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func NewLogger(env string) zerolog.Logger {
	level := zerolog.InfoLevel
	if strings.EqualFold(env, "dev") {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339Nano
	if strings.EqualFold(env, "dev") {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}
