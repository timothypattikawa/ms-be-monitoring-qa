package log

type Option struct {
	OutputPath string `default:"application.log"`                // OutputPath is output path of log file
	Level      string `option:"info|error|debug" default:"info"` // Level is log level of Logger
}

func NewDefaultOption() *Option {
	return &Option{
		OutputPath: "application.log",
		Level:      "info",
	}
}
