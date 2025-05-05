package auditlog

import (
	"io"
	"os"

	"github.com/rs/zerolog"
)

var logger zerolog.Logger

func Init(auditPath string) error {
	output := io.Discard
	if auditPath != "" {
		f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		output = f
	}

	logger = zerolog.New(output).With().
		Timestamp().
		Str("stream", "audit").
		Logger().
		Level(zerolog.InfoLevel)

	return nil
}

func Info() *zerolog.Event  { return logger.Info() }
func Warn() *zerolog.Event  { return logger.Warn() }
func Error() *zerolog.Event { return logger.Error() }
func Log() zerolog.Logger   { return logger }
