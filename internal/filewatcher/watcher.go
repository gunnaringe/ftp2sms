package filewatcher

import (
	"github.com/fsnotify/fsnotify"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type CallbackFunc func(phoneNumber, filePath, content string)

type FileWatcher struct {
	logger        *slog.Logger
	watcher       *fsnotify.Watcher
	callbacks     []CallbackFunc
	callbackMutex sync.Mutex
}

func NewFileWatcher(logger *slog.Logger) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fileWatcher := &FileWatcher{
		logger:  logger,
		watcher: watcher,
	}

	go fileWatcher.watchLoop()

	return fileWatcher, nil
}

func (fw *FileWatcher) OnUpdate(callback CallbackFunc) {
	fw.callbackMutex.Lock()
	fw.callbacks = append(fw.callbacks, callback)
	fw.callbackMutex.Unlock()
}

func (fw *FileWatcher) Watch(directory string) error {
	return filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			err := fw.watcher.Add(path)
			if err != nil {
				fw.logger.Warn("Failed to watch directory", "directory", path, "error", err)
				return err
			}
			fw.logger.Info("Watched directory", "directory", path)
		}

		return nil
	})
}

func (fw *FileWatcher) watchLoop() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) {
				if fileInfo, err := os.ReadDir(event.Name); err == nil && len(fileInfo) == 0 {
					// Skip directory if more than one level deep
					fw.watcher.Add(event.Name)
				} else {
					fw.processEvent(event.Name)
				}
			} else if event.Has(fsnotify.Write) {
				fw.processEvent(event.Name)
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}

func (fw *FileWatcher) processEvent(filePath string) {
	// TODO(gi): Fix this - "data" is not a constant
	relPath, _ := filepath.Rel("data", filePath)
	parts := strings.Split(relPath, string(filepath.Separator))

	if len(parts) < 2 {
		return
	}

	phoneNumber, filename := parts[0], parts[1]

	if fileInfo, err := os.ReadFile(filePath); err == nil && len(fileInfo) != 0 {
		if len(fileInfo) > 1000 {
			fw.logger.Warn("File is too big", "filename", filename, "size", len(fileInfo))
			return
		}
		content := string(fileInfo)
		fw.callbackMutex.Lock()
		for _, callback := range fw.callbacks {
			callback(phoneNumber, filePath, content)
		}
		fw.callbackMutex.Unlock()
	}
}
