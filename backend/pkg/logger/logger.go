// Package logger wraps zap so the rest of the codebase never imports zap directly.
package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the application logging port.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	With(fields ...Field) Logger
	Sync() error
}

// Field is a structured log attribute.
type Field = zap.Field

// Field constructors re-exported so callers stay decoupled from zap.
var (
	String   = zap.String
	Int      = zap.Int
	Int64    = zap.Int64
	Float64  = zap.Float64
	Bool     = zap.Bool
	Duration = zap.Duration
	Any      = zap.Any
	Err      = zap.Error
)

type zapLogger struct{ l *zap.Logger }

// New builds a logger. Development mode uses human-readable console output;
// anything else emits JSON for log shipping.
func New(env, level string) Logger {
	var encoderCfg zapcore.EncoderConfig
	var encoder zapcore.Encoder

	if env == "development" {
		encoderCfg = zap.NewDevelopmentEncoderConfig()
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoderCfg = zap.NewProductionEncoderConfig()
		encoderCfg.TimeKey = "ts"
		encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	core := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), parseLevel(level))
	return &zapLogger{l: zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))}
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func (z *zapLogger) Debug(msg string, f ...Field) { z.l.Debug(msg, f...) }
func (z *zapLogger) Info(msg string, f ...Field)  { z.l.Info(msg, f...) }
func (z *zapLogger) Warn(msg string, f ...Field)  { z.l.Warn(msg, f...) }
func (z *zapLogger) Error(msg string, f ...Field) { z.l.Error(msg, f...) }
func (z *zapLogger) Fatal(msg string, f ...Field) { z.l.Fatal(msg, f...) }
func (z *zapLogger) Sync() error                  { return z.l.Sync() }

func (z *zapLogger) With(f ...Field) Logger {
	return &zapLogger{l: z.l.With(f...)}
}
