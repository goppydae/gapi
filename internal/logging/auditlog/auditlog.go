package auditlog

import (
	"io"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logger  zerolog.Logger
	current io.Closer // currently open audit sink, closed on re-Init
)

func Init(auditPath string) error {
	// Close any previously opened audit sink first so repeated Init calls (e.g.
	// across tests or config reloads) don't leak file descriptors.
	if current != nil {
		_ = current.Close()
		current = nil
	}

	var output io.Writer = io.Discard
	if auditPath != "" {
		// lumberjack rotates the audit file so long-running daemons don't grow it
		// without bound.
		lj := &lumberjack.Logger{
			Filename:   auditPath,
			MaxSize:    100, // megabytes
			MaxBackups: 5,
			MaxAge:     30, // days
			Compress:   true,
		}
		output = lj
		current = lj
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
