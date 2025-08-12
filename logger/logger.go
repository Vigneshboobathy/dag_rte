package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var Logger *zap.Logger

func InitLogger(logFile string, level string) error {
	cfg := zap.NewProductionEncoderConfig()
	cfg.TimeKey = "time"
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder

	atom := zap.NewAtomicLevel()
	if err := atom.UnmarshalText([]byte(level)); err != nil {
		return err
	}

	// Open or create the log file
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	writeSyncer := zapcore.AddSync(file)
	encoder := zapcore.NewJSONEncoder(cfg)

	core := zapcore.NewCore(encoder, writeSyncer, atom)
	Logger = zap.New(core, zap.AddCaller())

	return nil
}
