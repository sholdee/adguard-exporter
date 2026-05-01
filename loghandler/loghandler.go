package loghandler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/sholdee/adguard-exporter/metrics"
)

const logFingerprintBytes = 4096

type LogWatcher interface {
	Run(context.Context)
	Close() error
}

type LogHandler struct {
	logFilePath      string
	metricsCollector *metrics.MetricsCollector
	lastPosition     int64
	lastFingerprint  []byte
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
	if err := lh.ensureReadPosition(file, info.Size()); err != nil {
		log.Printf("Error verifying log file position: %v", err)
		lh.healthStatus = false
		return
	}

	_, err = file.Seek(lh.lastPosition, io.SeekStart)
	if err != nil {
		log.Printf("Error seeking in log file: %v", err)
		lh.healthStatus = false
		return
	}

	reader := bufio.NewReader(file)
	position := lh.lastPosition
	for {
		lineStart := position
		line, err := reader.ReadBytes('\n')
		if err == nil {
			lh.processLine(line)
			position += int64(len(line))
			continue
		}

		if err == io.EOF {
			if len(line) > 0 {
				position = lineStart
			}
			break
		}

		log.Printf("Error reading log file: %v", err)
		lh.healthStatus = false
		return
	}

	lh.lastPosition = position
	if err := lh.refreshFingerprint(file); err != nil {
		log.Printf("Error recording log file fingerprint: %v", err)
		lh.healthStatus = false
		return
	}
	lh.healthStatus = true
}

func (lh *LogHandler) ensureReadPosition(file *os.File, size int64) error {
	if lh.lastPosition == 0 {
		return nil
	}
	if size < lh.lastPosition {
		log.Printf("Log file was truncated; resetting read position from %d to 0", lh.lastPosition)
		lh.resetReadPosition()
		return nil
	}
	if len(lh.lastFingerprint) == 0 {
		return nil
	}

	fingerprint, err := readLogFingerprint(file, lh.lastPosition)
	if err != nil {
		return err
	}
	if !bytes.Equal(fingerprint, lh.lastFingerprint) {
		log.Printf("Log file changed before read position; resetting read position from %d to 0", lh.lastPosition)
		lh.resetReadPosition()
	}
	return nil
}

func (lh *LogHandler) refreshFingerprint(file *os.File) error {
	fingerprint, err := readLogFingerprint(file, lh.lastPosition)
	if err != nil {
		return err
	}
	lh.lastFingerprint = fingerprint
	return nil
}

func (lh *LogHandler) resetReadPosition() {
	lh.lastPosition = 0
	lh.lastFingerprint = nil
}

func readLogFingerprint(file *os.File, position int64) ([]byte, error) {
	if position <= 0 {
		return nil, nil
	}

	length := int64(logFingerprintBytes)
	if position < length {
		length = position
	}

	fingerprint := make([]byte, length)
	n, err := file.ReadAt(fingerprint, position-length)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return fingerprint[:n], nil
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

func (lh *LogHandler) WatchLogFile(ctx context.Context) error {
	watcher, err := lh.NewLogWatcher()
	if err != nil {
		return err
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.Printf("Error closing log file watcher: %v", err)
		}
	}()

	watcher.Run(ctx)
	return nil
}

func (lh *LogHandler) NewLogWatcher() (LogWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		lh.setHealth(false)
		return nil, fmt.Errorf("create log file watcher: %w", err)
	}

	if err := watcher.Add(filepath.Dir(lh.logFilePath)); err != nil {
		if closeErr := watcher.Close(); closeErr != nil {
			log.Printf("Error closing log file watcher after setup failure: %v", closeErr)
		}
		lh.setHealth(false)
		return nil, fmt.Errorf("watch log file directory: %w", err)
	}

	return &fsLogWatcher{handler: lh, watcher: watcher}, nil
}

type fsLogWatcher struct {
	handler *LogHandler
	watcher *fsnotify.Watcher
}

func (w *fsLogWatcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handler.handleWatchEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.handler.handleWatchError(err)
		}
	}
}

func (w *fsLogWatcher) Close() error {
	return w.watcher.Close()
}

func (lh *LogHandler) handleWatchEvent(event fsnotify.Event) {
	if filepath.Base(event.Name) != filepath.Base(lh.logFilePath) {
		return
	}
	if event.Op&fsnotify.Write == fsnotify.Write {
		lh.processNewLines()
	} else if event.Op&fsnotify.Create == fsnotify.Create {
		log.Printf("Log file created: %s", event.Name)
		lh.resetForCreatedFile()
		lh.processNewLines()
	}
}

func (lh *LogHandler) handleWatchError(err error) {
	lh.setHealth(false)
	log.Println("error:", err)
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
	lh.resetReadPosition()
	lh.healthStatus = true
}
