package log

import (
	"go.uber.org/zap"
)

var (
	logger *zap.Logger
	sugar  *zap.SugaredLogger
)

func Init() error {
	built, err := zap.NewProductionConfig().Build()
	if err != nil {
		// A logging setup failure must not take the process down; fall back to a
		// no-op logger and return the error to callers that care.
		logger = zap.NewNop()
		sugar = logger.Sugar()
		return err
	}
	logger = built
	sugar = logger.Sugar()
	return nil
}

func init() {
	_ = Init()
}

// Info logs a message with optional key/value pairs, for example:
// log.Info("server started", "port", 8080).
func Info(msg string, fields ...interface{}) {
	sugar.Infow(msg, fields...)
}

func Infof(format string, args ...interface{}) {
	sugar.Infof(format, args...)
}

func Error(msg string, fields ...interface{}) {
	sugar.Errorw(msg, fields...)
}

func Errorf(format string, args ...interface{}) {
	sugar.Errorf(format, args...)
}

func Warn(msg string, fields ...interface{}) {
	sugar.Warnw(msg, fields...)
}

func Warnf(format string, args ...interface{}) {
	sugar.Warnf(format, args...)
}

func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}
