package utils

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

var sourceRootDir string

func init() {
	_, file, _, _ := runtime.Caller(0)
	if strings.HasSuffix(file, "internal/shared/utils/runtime.go") {
		sourceRootDir = strings.TrimSuffix(file, "internal/shared/utils/runtime.go")
	}

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
}

func Root() string { return sourceRootDir }

func GetSource(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}

	return fmt.Sprintf("%s:L%d", strings.TrimPrefix(file, Root()), line)
}

var hostname string

func GetHostname() string { return hostname }
