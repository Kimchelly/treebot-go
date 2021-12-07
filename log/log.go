package log

import (
	"fmt"

	"go.uber.org/zap"
)

var Logger *zap.SugaredLogger

func init() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %s", err))
	}
	Logger = logger.Sugar()
}
