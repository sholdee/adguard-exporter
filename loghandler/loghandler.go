package loghandler

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/sholdee/adguard-exporter/metrics"
)

type LogHandler struct {
	logFilePath      string
	metricsCollector *metrics.MetricsCollector
	lastPosition     int64
	healthStatus     bool
	fileExists       bool
	lock             sync.Mutex
}

func NewLogHandler(logFilePath string, metricsCollector *metrics.MetricsCollector) *LogHandler {
	lh := &LogHandler{
		logFilePath:      logFilePath,
		metricsCollector: metricsCollector,
		healthStatus:     true,
		fileExists:       false,
	}
	lh.initialLoad()
	return lh
}

func (lh *LogHandler) initialLoad() {
	if _, err := os.Stat(lh.logFilePath); err == nil {
		log.Printf("Loading existing log file: %s", lh.logFilePath)
		lh.setFileExists(true)
		lh.processNewLines()
	} else {
		log.Printf("Waiting for log file: %s", lh.logFilePath)
	}
}

func (lh *LogHandler) processNewLines() {
	lh.lock.Lock()
	defer lh.lock.Unlock()

	file, err := os.Open(lh.logFilePath)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		lh.healthStatus = false
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		log.Printf("Error stating log file: %v", err)
		lh.healthStatus = false
		return
	}
	if info.Size() < lh.lastPosition {
		log.Printf("Log file was truncated; resetting read position from %d to 0", lh.lastPosition)
		lh.lastPosition = 0
	}

	_, err = file.Seek(lh.lastPosition, io.SeekStart)
	if err != nil {
		log.Printf("Error seeking in log file: %v", err)
		lh.healthStatus = false
		return
	}

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lh.processLine(line)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error reading log file: %v", err)
			lh.healthStatus = false
			return
		}
	}

	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		log.Printf("Error recording log file position: %v", err)
		lh.healthStatus = false
		return
	}
	lh.lastPosition = position
	lh.healthStatus = true
}

func (lh *LogHandler) processLine(line []byte) {
	if len(line) == 0 {
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(line, &data); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		return
	}
	// Ensure Upstream is present and not empty
	if upstream, ok := data["Upstream"]; !ok || upstream == "" {
		data["Upstream"] = "unknown"
	}
	lh.metricsCollector.UpdateMetrics(data)
}

func (lh *LogHandler) WatchLogFile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Check if the event is for our specific file
				if filepath.Base(event.Name) != filepath.Base(lh.logFilePath) {
					continue // Ignore events for other files
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					lh.processNewLines()
				} else if event.Op&fsnotify.Create == fsnotify.Create {
					log.Printf("Log file created: %s", event.Name)
					lh.resetForCreatedFile()
					lh.processNewLines()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				lh.setHealth(false)
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(filepath.Dir(lh.logFilePath))
	if err != nil {
		_ = watcher.Close()
		log.Fatal(err)
	}
	defer watcher.Close()
	<-done
}

func (lh *LogHandler) IsHealthy() bool {
	lh.lock.Lock()
	defer lh.lock.Unlock()
	return lh.healthStatus
}

func (lh *LogHandler) setHealth(healthy bool) {
	lh.lock.Lock()
	defer lh.lock.Unlock()
	lh.healthStatus = healthy
}

func (lh *LogHandler) setFileExists(exists bool) {
	lh.lock.Lock()
	defer lh.lock.Unlock()
	lh.fileExists = exists
}

func (lh *LogHandler) resetForCreatedFile() {
	lh.lock.Lock()
	defer lh.lock.Unlock()
	lh.fileExists = true
	lh.lastPosition = 0
	lh.healthStatus = true
}
