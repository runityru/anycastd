package gobgpv4

import (
	"context"
	"log/slog"

	log "github.com/sirupsen/logrus"
)

// slogBridge is a slog.Handler that forwards every slog record to a logrus
// *log.Logger. GoBGP v4's server only accepts an *slog.Logger, so the bridge
// lets a v4 server be observed through the same logrus sink and formatter as
// the rest of the process and the v3 engine. Logrus's own level filter decides
// whether an entry is emitted.
type slogBridge struct {
	logger *log.Logger
}

var _ slog.Handler = (*slogBridge)(nil)

// Enabled always returns true; logrus filters by its configured level.
func (h *slogBridge) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle forwards a single slog record to the logrus logger, mapping the slog
// level onto the nearest logrus level and copying attrs into logrus fields.
func (h *slogBridge) Handle(_ context.Context, r slog.Record) error {
	lvl := log.InfoLevel
	switch r.Level {
	case slog.LevelDebug:
		lvl = log.DebugLevel
	case slog.LevelWarn:
		lvl = log.WarnLevel
	case slog.LevelError:
		lvl = log.ErrorLevel
	}

	fields := log.Fields{}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})

	h.logger.WithFields(fields).Log(lvl, r.Message)
	return nil
}

// WithAttrs returns a bridge that forwards attrs to the same logrus logger.
func (h *slogBridge) WithAttrs(_ []slog.Attr) slog.Handler { return h }

// WithGroup returns the bridge unchanged (groups are not modelled on logrus).
func (h *slogBridge) WithGroup(_ string) slog.Handler { return h }
