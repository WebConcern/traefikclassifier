package lib

import (
	"log"
	"os"
	"sync"
	"time"
)

// fileStamp identifies one version of a file on disk.
type fileStamp struct {
	modTime time.Time
	size    int64
}

func (s fileStamp) equal(o fileStamp) bool {
	return s.size == o.size && s.modTime.Equal(o.modTime)
}

func statFile(path string) fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{modTime: info.ModTime(), size: info.Size()}
}

// watchedFile is a data file that is reloaded when it changes on disk.
// load must only swap in new data when it succeeds, so a broken or missing
// file never replaces data that is already loaded.
type watchedFile struct {
	path  string
	stamp fileStamp
	load  func(path string) error
}

var (
	watchedMu     sync.Mutex
	watchedFiles  = map[string]*watchedFile{}
	refresherOnce sync.Once
)

// watchFile registers a data file under key, replacing any earlier registration.
// stamp must be taken before the initial load, so a write that races with it is
// picked up on the next refresh.
func watchFile(key, path string, stamp fileStamp, load func(path string) error) {
	watchedMu.Lock()
	defer watchedMu.Unlock()
	watchedFiles[key] = &watchedFile{path: path, stamp: stamp, load: load}
}

// RefreshFiles reloads watched data files that changed on disk.
func RefreshFiles() {
	watchedMu.Lock()
	defer watchedMu.Unlock()

	for key, wf := range watchedFiles {
		stamp := statFile(wf.path)
		// A missing file keeps the previous data; it is loaded once it reappears.
		if stamp.modTime.IsZero() || stamp.equal(wf.stamp) {
			continue
		}
		wf.stamp = stamp
		if err := wf.load(wf.path); err != nil {
			log.Printf("[traefik-classifier] ERROR: reloading %s from %s failed, keeping previous data: %v", key, wf.path, err)
			continue
		}
		log.Printf("[traefik-classifier] Reloaded %s from %s", key, wf.path)
	}
}

// StartRefresher checks watched files for changes every interval. Only the first
// call starts the background loop; it runs for the lifetime of the process.
func StartRefresher(interval time.Duration) {
	if interval <= 0 {
		return
	}
	refresherOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				RefreshFiles()
			}
		}()
	})
}
