package announcer

import (
	gobgpLog "github.com/osrg/gobgp/v3/pkg/log"
	log "github.com/sirupsen/logrus"
)

type Logger struct {
	Logger *log.Logger
}

func (l *Logger) Panic(msg string, fields gobgpLog.Fields) {
	l.Logger.WithFields(log.Fields(fields)).Panic(msg)
}

func (l *Logger) Fatal(msg string, fields gobgpLog.Fields) {
	l.Logger.WithFields(log.Fields(fields)).Fatal(msg)
}

func (l *Logger) Error(msg string, fields gobgpLog.Fields) {
	l.Logger.WithFields(log.Fields(fields)).Error(msg)
}

func (l *Logger) Warn(msg string, fields gobgpLog.Fields) {
	l.Logger.WithFields(log.Fields(fields)).Warn(msg)
}

func (l *Logger) Info(msg string, fields gobgpLog.Fields) {
	l.Logger.WithFields(log.Fields(fields)).Info(msg)
}

func (l *Logger) Debug(msg string, fields gobgpLog.Fields) {
	l.Logger.WithFields(log.Fields(fields)).Debug(msg)
}

func (l *Logger) SetLevel(level gobgpLog.LogLevel) {
	l.Logger.SetLevel(log.Level(level))
}

func (l *Logger) GetLevel() gobgpLog.LogLevel {
	return gobgpLog.LogLevel(l.Logger.GetLevel())
}
