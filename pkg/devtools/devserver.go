package devtools

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// DevServerConfig holds dev server configuration
type DevServerConfig struct {
	Port         int
	WatchDir     string
	BuildCmd     string
	RunCmd       string
	ExcludePatterns []string
}

// DevServer is a development server with hot-reload
type DevServer struct {
	config     DevServerConfig
	process    *os.Process
	fileHashes map[string]string
	mu         sync.RWMutex
	stopCh     chan struct{}
	stopOnce   sync.Once
	isRunning  bool
}

// NewDevServer creates a new dev server
func NewDevServer(config DevServerConfig) *DevServer {
	if config.BuildCmd == "" {
		config.BuildCmd = "go build -o ./bin/agent ./cmd/..."
	}
	if config.RunCmd == "" {
		config.RunCmd = "./bin/agent"
	}
	if len(config.ExcludePatterns) == 0 {
		config.ExcludePatterns = []string{
			"*.test",
			"*_test.go",
			".git",
			"bin",
			"vendor",
		}
	}

	return &DevServer{
		config:     config,
		fileHashes: make(map[string]string),
		stopCh:     make(chan struct{}),
	}
}

// Start starts the development server
func (ds *DevServer) Start() error {
	fmt.Printf("Starting dev server on port %d\n", ds.config.Port)
	fmt.Printf("Watching directory: %s\n", ds.config.WatchDir)

	// Initial build
	if err := ds.build(); err != nil {
		return fmt.Errorf("initial build failed: %w", err)
	}

	// Start the application
	if err := ds.startApp(); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	// Start file watcher
	go ds.watchFiles()

	// Start HTTP server for dev tools
	go ds.startHTTPServer()

	// Wait for stop signal
	<-ds.stopCh

	ds.stopApp()
	return nil
}

// Stop stops the development server
func (ds *DevServer) Stop() {
	ds.stopOnce.Do(func() {
		close(ds.stopCh)
	})
}

func (ds *DevServer) build() error {
	fmt.Println("Building...")
	cmd := exec.Command("sh", "-c", ds.config.BuildCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("Build complete")
	return nil
}

func (ds *DevServer) startApp() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.isRunning {
		ds.stopAppLocked()
	}

	cmd := exec.Command("sh", "-c", ds.config.RunCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	ds.process = cmd.Process
	ds.isRunning = true

	fmt.Printf("Application started (PID: %d)\n", ds.process.Pid)
	return nil
}

func (ds *DevServer) stopApp() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.stopAppLocked()
}

func (ds *DevServer) stopAppLocked() {
	if ds.process != nil && ds.isRunning {
		ds.process.Signal(os.Interrupt)
		time.Sleep(100 * time.Millisecond)
		ds.process.Kill()
		ds.isRunning = false
		fmt.Println("Application stopped")
	}
}

func (ds *DevServer) watchFiles() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Build initial hash map
	ds.scanFiles()

	for {
		select {
		case <-ds.stopCh:
			return
		case <-ticker.C:
			if ds.hasChanges() {
				fmt.Println("\nFile changes detected, rebuilding...")
				if err := ds.build(); err != nil {
					fmt.Printf("Build error: %v\n", err)
					continue
				}
				if err := ds.startApp(); err != nil {
					fmt.Printf("Restart error: %v\n", err)
				}
			}
		}
	}
}

func (ds *DevServer) scanFiles() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	filepath.Walk(ds.config.WatchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			for _, pattern := range ds.config.ExcludePatterns {
				if matched, _ := filepath.Match(pattern, info.Name()); matched {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Only watch Go files
		if filepath.Ext(path) != ".go" {
			return nil
		}

		// Check exclude patterns
		for _, pattern := range ds.config.ExcludePatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				return nil
			}
		}

		hash, err := ds.hashFile(path)
		if err != nil {
			return nil
		}

		ds.fileHashes[path] = hash
		return nil
	})
}

func (ds *DevServer) hasChanges() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	changed := false

	filepath.Walk(ds.config.WatchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			for _, pattern := range ds.config.ExcludePatterns {
				if matched, _ := filepath.Match(pattern, info.Name()); matched {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		for _, pattern := range ds.config.ExcludePatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				return nil
			}
		}

		hash, err := ds.hashFile(path)
		if err != nil {
			return nil
		}

		oldHash, exists := ds.fileHashes[path]
		if !exists || oldHash != hash {
			ds.fileHashes[path] = hash
			changed = true
		}

		return nil
	})

	return changed
}

func (ds *DevServer) hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (ds *DevServer) startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>SYNTOR Dev Server</title>
    <style>
        body { font-family: sans-serif; margin: 40px; }
        h1 { color: #333; }
        .status { padding: 10px; margin: 10px 0; border-radius: 5px; }
        .healthy { background: #d4edda; }
        .unhealthy { background: #f8d7da; }
    </style>
</head>
<body>
    <h1>SYNTOR Development Server</h1>
    <div class="status healthy">
        <strong>Status:</strong> Running
    </div>
    <p><strong>Port:</strong> %d</p>
    <p><strong>Watch Directory:</strong> %s</p>
    <h2>Endpoints</h2>
    <ul>
        <li><a href="/health">/health</a> - Health check</li>
        <li><a href="/reload">/reload</a> - Force reload</li>
        <li><a href="/stop">/stop</a> - Stop server</li>
    </ul>
</body>
</html>`, ds.config.Port, ds.config.WatchDir)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ds.mu.RLock()
		running := ds.isRunning
		ds.mu.RUnlock()

		if running {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"healthy","running":true}`)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"unhealthy","running":false}`)
		}
	})

	mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			if err := ds.build(); err != nil {
				fmt.Printf("Build error: %v\n", err)
				return
			}
			ds.startApp()
		}()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"reloading"}`)
	})

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"stopping"}`)
		go ds.Stop()
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", ds.config.Port),
		Handler: mux,
	}

	server.ListenAndServe()
}

// HotReloader provides hot-reload functionality
type HotReloader struct {
	watchDir     string
	onChange     func() error
	debounceMs   int
	fileHashes   map[string]string
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// HotReloaderConfig holds hot reloader configuration
type HotReloaderConfig struct {
	WatchDir   string
	DebounceMs int
	OnChange   func() error
}

// NewHotReloader creates a new hot reloader
func NewHotReloader(config HotReloaderConfig) *HotReloader {
	if config.DebounceMs <= 0 {
		config.DebounceMs = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &HotReloader{
		watchDir:   config.WatchDir,
		onChange:   config.OnChange,
		debounceMs: config.DebounceMs,
		fileHashes: make(map[string]string),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start starts the hot reloader
func (hr *HotReloader) Start() {
	go hr.watchLoop()
}

// Stop stops the hot reloader
func (hr *HotReloader) Stop() {
	hr.cancel()
}

// IsEnabled returns whether hot reload is enabled
func (hr *HotReloader) IsEnabled() bool {
	return hr.onChange != nil
}

func (hr *HotReloader) watchLoop() {
	ticker := time.NewTicker(time.Duration(hr.debounceMs) * time.Millisecond)
	defer ticker.Stop()

	// Initial scan
	hr.scanDirectory()

	for {
		select {
		case <-hr.ctx.Done():
			return
		case <-ticker.C:
			if hr.detectChanges() {
				if hr.onChange != nil {
					if err := hr.onChange(); err != nil {
						fmt.Printf("Hot reload error: %v\n", err)
					}
				}
			}
		}
	}
}

func (hr *HotReloader) scanDirectory() {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	filepath.Walk(hr.watchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if filepath.Ext(path) == ".go" {
			hash, _ := hashFileContent(path)
			hr.fileHashes[path] = hash
		}
		return nil
	})
}

func (hr *HotReloader) detectChanges() bool {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	changed := false

	filepath.Walk(hr.watchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if filepath.Ext(path) == ".go" {
			hash, _ := hashFileContent(path)
			if oldHash, exists := hr.fileHashes[path]; !exists || oldHash != hash {
				hr.fileHashes[path] = hash
				changed = true
			}
		}
		return nil
	})

	return changed
}

func hashFileContent(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), nil
}
