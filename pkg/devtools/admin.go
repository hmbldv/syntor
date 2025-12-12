package devtools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"time"
)

// HealthStatus represents system health status
type HealthStatus struct {
	Overall    string            `json:"overall"`
	Components map[string]string `json:"components"`
	Timestamp  time.Time         `json:"timestamp"`
}

// HealthChecker checks system health
type HealthChecker struct {
	checks map[string]func() (string, error)
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	hc := &HealthChecker{
		checks: make(map[string]func() (string, error)),
	}

	// Register default checks
	hc.RegisterCheck("runtime", hc.checkRuntime)
	hc.RegisterCheck("memory", hc.checkMemory)
	hc.RegisterCheck("goroutines", hc.checkGoroutines)

	return hc
}

// RegisterCheck registers a health check
func (hc *HealthChecker) RegisterCheck(name string, check func() (string, error)) {
	hc.checks[name] = check
}

// Check runs all health checks
func (hc *HealthChecker) Check() HealthStatus {
	status := HealthStatus{
		Overall:    "healthy",
		Components: make(map[string]string),
		Timestamp:  time.Now(),
	}

	for name, check := range hc.checks {
		result, err := check()
		if err != nil {
			status.Components[name] = "unhealthy: " + err.Error()
			status.Overall = "unhealthy"
		} else {
			status.Components[name] = result
		}
	}

	return status
}

func (hc *HealthChecker) checkRuntime() (string, error) {
	return fmt.Sprintf("Go %s on %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH), nil
}

func (hc *HealthChecker) checkMemory() (string, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Check if memory usage is reasonable
	if m.Alloc > 1024*1024*1024 { // 1GB
		return fmt.Sprintf("%.2f MB (high)", float64(m.Alloc)/1024/1024), nil
	}

	return fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024), nil
}

func (hc *HealthChecker) checkGoroutines() (string, error) {
	count := runtime.NumGoroutine()

	if count > 10000 {
		return fmt.Sprintf("%d (high)", count), fmt.Errorf("too many goroutines")
	}

	return fmt.Sprintf("%d", count), nil
}

// HTTPHandler returns an HTTP handler for health checks
func (hc *HealthChecker) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := hc.Check()

		w.Header().Set("Content-Type", "application/json")

		if status.Overall != "healthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(status)
	})
}

// SystemStatus represents overall system status
type SystemStatus struct {
	Agents []AgentStatus `json:"agents"`
	Tasks  []TaskStatus  `json:"tasks"`
}

// AgentStatus represents agent status
type AgentStatus struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	LastSeen   time.Time `json:"last_seen"`
	TaskCount  int       `json:"task_count"`
	Load       float64   `json:"load"`
}

// TaskStatus represents task status
type TaskStatus struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	AgentID   string    `json:"agent_id"`
	CreatedAt time.Time `json:"created_at"`
	Duration  string    `json:"duration,omitempty"`
}

// StatusMonitor monitors system status
type StatusMonitor struct {
	registryAddr string
}

// NewStatusMonitor creates a new status monitor
func NewStatusMonitor() *StatusMonitor {
	return &StatusMonitor{
		registryAddr: os.Getenv("REDIS_ADDR"),
	}
}

// GetStatus returns current system status
func (sm *StatusMonitor) GetStatus() SystemStatus {
	// In a real implementation, this would query the registry
	// For now, return empty status
	return SystemStatus{
		Agents: []AgentStatus{},
		Tasks:  []TaskStatus{},
	}
}

// HTTPHandler returns an HTTP handler for status
func (sm *StatusMonitor) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := sm.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})
}

// Profiler provides profiling tools
type Profiler struct {
	outputDir string
}

// NewProfiler creates a new profiler
func NewProfiler() *Profiler {
	return &Profiler{
		outputDir: "profiles",
	}
}

// CPUProfile runs a CPU profile
func (p *Profiler) CPUProfile(duration int) error {
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s/cpu_%s.prof", p.outputDir, time.Now().Format("20060102_150405"))
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	fmt.Printf("Starting CPU profile for %d seconds...\n", duration)
	fmt.Printf("Output: %s\n", filename)

	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return err
	}

	time.Sleep(time.Duration(duration) * time.Second)
	pprof.StopCPUProfile()
	f.Close()

	fmt.Println("CPU profile complete")
	fmt.Printf("Analyze with: go tool pprof %s\n", filename)

	return nil
}

// MemProfile captures a memory profile
func (p *Profiler) MemProfile() error {
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s/mem_%s.prof", p.outputDir, time.Now().Format("20060102_150405"))
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	runtime.GC() // Run GC before capturing heap profile
	if err := pprof.WriteHeapProfile(f); err != nil {
		return err
	}

	fmt.Printf("Memory profile saved: %s\n", filename)
	fmt.Printf("Analyze with: go tool pprof %s\n", filename)

	return nil
}

// Trace captures an execution trace
func (p *Profiler) Trace(duration int) error {
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s/trace_%s.out", p.outputDir, time.Now().Format("20060102_150405"))
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	fmt.Printf("Starting trace for %d seconds...\n", duration)
	fmt.Printf("Output: %s\n", filename)

	if err := trace.Start(f); err != nil {
		f.Close()
		return err
	}

	time.Sleep(time.Duration(duration) * time.Second)
	trace.Stop()
	f.Close()

	fmt.Println("Trace complete")
	fmt.Printf("Analyze with: go tool trace %s\n", filename)

	return nil
}

// HTTPHandler returns an HTTP handler for profiling
func (p *Profiler) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
		// Use default pprof handlers
		http.DefaultServeMux.ServeHTTP(w, r)
	})

	return mux
}

// AdminServer provides administrative HTTP endpoints
type AdminServer struct {
	port          int
	healthChecker *HealthChecker
	statusMonitor *StatusMonitor
	profiler      *Profiler
}

// AdminServerConfig holds admin server configuration
type AdminServerConfig struct {
	Port int
}

// NewAdminServer creates a new admin server
func NewAdminServer(config AdminServerConfig) *AdminServer {
	if config.Port <= 0 {
		config.Port = 9090
	}

	return &AdminServer{
		port:          config.Port,
		healthChecker: NewHealthChecker(),
		statusMonitor: NewStatusMonitor(),
		profiler:      NewProfiler(),
	}
}

// Start starts the admin server
func (as *AdminServer) Start() error {
	mux := http.NewServeMux()

	mux.Handle("/health", as.healthChecker.HTTPHandler())
	mux.Handle("/status", as.statusMonitor.HTTPHandler())
	mux.Handle("/debug/pprof/", as.profiler.HTTPHandler())

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>SYNTOR Admin</title>
    <style>
        body { font-family: sans-serif; margin: 40px; }
        h1 { color: #333; }
        ul { list-style: none; padding: 0; }
        li { margin: 10px 0; }
        a { color: #007bff; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <h1>SYNTOR Admin Interface</h1>
    <h2>Endpoints</h2>
    <ul>
        <li><a href="/health">/health</a> - Health check</li>
        <li><a href="/status">/status</a> - System status</li>
        <li><a href="/debug/pprof/">/debug/pprof/</a> - Profiling</li>
    </ul>
</body>
</html>`)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", as.port),
		Handler: mux,
	}

	fmt.Printf("Admin server starting on port %d\n", as.port)
	return server.ListenAndServe()
}
