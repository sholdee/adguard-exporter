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
		lh.fileExists = true
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

	_, err = file.Seek(lh.lastPosition, io.SeekStart)
	if err != nil {
		log.Printf("Error seeking in log file: %v", err)
		lh.healthStatus = false
		return
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var data map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &data); err != nil {
			log.Printf("Error decoding JSON: %v", err)
			continue
		}
		// Ensure Upstream is present and not empty
		if upstream, ok := data["Upstream"]; !ok || upstream == "" {
			data["Upstream"] = "unknown"
		}
		lh.metricsCollector.UpdateMetrics(data)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading log file: %v", err)
		lh.healthStatus = false
		return
	}

	lh.lastPosition, _ = file.Seek(0, io.SeekCurrent)
	lh.healthStatus = true
}

func (lh *LogHandler) WatchLogFile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

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
					lh.fileExists = true
					lh.lastPosition = 0
					lh.processNewLines()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				lh.healthStatus = false
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(filepath.Dir(lh.logFilePath))
	if err != nil {
		log.Fatal(err)
	}
	<-done
}

func (lh *LogHandler) IsHealthy() bool {
	lh.lock.Lock()
	defer lh.lock.Unlock()
	return lh.healthStatus
}
