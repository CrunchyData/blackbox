package blackbox

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudfoundry/blackbox/syslog"
	"github.com/tedsuo/ifrit/grouper"
)

const POLL_INTERVAL = 5 * time.Second

type fileWatcher struct {
	logger *log.Logger

	sourceDir          string
	logSuffix          string
	dynamicGroupClient grouper.DynamicClient
	drainerFactory     syslog.DrainerFactory
}

func NewFileWatcher(
	logger *log.Logger,
	sourceDir string,
	logSuffix string,
	dynamicGroupClient grouper.DynamicClient,
	drainerFactory syslog.DrainerFactory,
) *fileWatcher {
	return &fileWatcher{
		logger:             logger,
		sourceDir:          sourceDir,
		logSuffix:          logSuffix,
		dynamicGroupClient: dynamicGroupClient,
		drainerFactory:     drainerFactory,
	}
}

func (f *fileWatcher) Watch() {
	for {
		logDirs, err := ioutil.ReadDir(f.sourceDir)
		if err != nil {
			f.logger.Fatalf("could not list directories in source dir: %s\n", err)
		}

		for _, logDir := range logDirs {
			tag := logDir.Name()
			tagDirPath := filepath.Join(f.sourceDir, tag)

			fileInfo, err := os.Stat(tagDirPath)
			if err != nil {
				f.logger.Fatalf("failed to determine if path is directory: %s\n", err)
			}

			if !fileInfo.IsDir() {
				continue
			}

			f.findLogsToWatch(tag, tagDirPath, fileInfo)

		}

		time.Sleep(POLL_INTERVAL)
	}
}

func (f *fileWatcher) findLogsToWatch(tag string, filePath string, file os.FileInfo) {
	if !file.IsDir() {
		if strings.HasSuffix(file.Name(), f.logSuffix) {
			if _, found := f.dynamicGroupClient.Get(filePath); !found {
				f.dynamicGroupClient.Inserter() <- f.memberForFile(filePath)
			}
		}
		return
	}

	dirContents, err := ioutil.ReadDir(filePath)
	if err != nil {
		f.logger.Printf("skipping log dir '%s' (could not list files): %s\n", tag, err)
		return
	}

	for _, content := range dirContents {
		currentFilePath := filepath.Join(filePath, content.Name())
		f.findLogsToWatch(tag, currentFilePath, content)
	}
}

func (f *fileWatcher) memberForFile(logfilePath string) grouper.Member {
	drainer, err := f.drainerFactory.NewDrainer()
	if err != nil {
		f.logger.Fatalf("could not drain to syslog: %s\n", err)
	}

	logfileDir := filepath.Dir(logfilePath)

	tag, err := filepath.Rel(f.sourceDir, logfileDir)
	if err != nil {
		f.logger.Fatalf("could not compute tag from file path %s: %s\n", logfilePath, err)
	}

	tailer := &Tailer{
		Path:    logfilePath,
		Tag:     tag,
		Drainer: drainer,
	}

	return grouper.Member{tailer.Path, tailer}
}
