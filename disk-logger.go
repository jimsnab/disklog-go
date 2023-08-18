package disklog

import (
	"os"
	"path"
	"strings"
	"sync"

	"github.com/jimsnab/go-lane"
)

type (
	diskLogger struct {
		mu sync.Mutex
		l lane.Lane
		basePath string
		logs map[string]*os.File
	}
)


func newDiskLogger(l lane.Lane, basePath string) (dl *diskLogger) {
	dl = &diskLogger{
		l: l,
		basePath: basePath,
		logs: map[string]*os.File{},
	}
	return
}

func (dl *diskLogger) processMessage(logName, logLine string) (err error) {
	targetPath := path.Join(dl.basePath, logName + ".log")
	fh, exists := dl.logs[targetPath];
	if !exists {
		fh, err = os.OpenFile(targetPath, os.O_APPEND, 0666)
		if err != nil {
			dl.l.Errorf("can't open %s: %v", targetPath, err)
			return
		}
		dl.logs[targetPath] = fh
	}

	if !strings.HasSuffix(logLine, "\n") {
		logLine = logLine + "\n"
	}

	fh.WriteString(logLine)
	return
}

func (dl *diskLogger) flushAll() {
	for name,log := range dl.logs {
		err := log.Sync()
		if err != nil {
			dl.l.Errorf("commit error for %s: %v", name, err)
		}
	}
}

func (dl *diskLogger) closeAll() {
	for name,log := range dl.logs {
		err := log.Close()
		if err != nil {
			dl.l.Errorf("error closing %s: %v", name, err)
		}
	}
	dl.logs = map[string]*os.File{}
}