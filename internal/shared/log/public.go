package log

import (
	"sync"
)

var logInstance Logger
var once sync.Once

// GetLogger will lazy init Logger interface to be used publicly
func GetLogger() Logger {
	once.Do(func() {
		conf := NewDefaultOption()

		// if configs.GetConfig().IsProduction() {
		// 	if configs.GetConfig().LogConfig.OutputPath != "" {
		// 		conf.OutputPath = configs.GetConfig().LogConfig.OutputPath
		// 	}
		// }

		conf.Level = "debug"

		logInstance, _ = NewLogger(conf)
	})

	return logInstance
}
