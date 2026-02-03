package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ParallelExecutor provides advanced parallel execution strategies.
type ParallelExecutor struct {
	manager      *Manager
	maxWorkers   int
	aggregator   ResultAggregator
}

// ResultAggregator defines how to combine results from parallel execution.
type ResultAggregator interface {
	Aggregate(results map[string]*Result) (*Result, error)
}

// NewParallelExecutor creates a parallel executor.
func NewParallelExecutor(manager *Manager, maxWorkers int) *ParallelExecutor {
	return &ParallelExecutor{
		manager:    manager,
		maxWorkers: maxWorkers,
		aggregator: &DefaultAggregator{},
	}
}

// WithAggregator sets a custom result aggregator.
func (pe *ParallelExecutor) WithAggregator(agg ResultAggregator) *ParallelExecutor {
	pe.aggregator = agg
	return pe
}

// ExecuteAll runs all agents in parallel with worker pool control.
func (pe *ParallelExecutor) ExecuteAll(ctx context.Context, requests []SpawnRequest) (*ParallelResult, error) {
	start := time.Now()

	// Create a worker pool
	workCh := make(chan SpawnRequest, len(requests))
	resultCh := make(chan workerResult, len(requests))

	// Start workers
	var wg sync.WaitGroup
	workers := pe.maxWorkers
	if workers > len(requests) {
		workers = len(requests)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go pe.worker(ctx, &wg, workCh, resultCh)
	}

	// Send work
	for _, req := range requests {
		workCh <- req
	}
	close(workCh)

	// Wait for completion
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	parallelResult := &ParallelResult{
		Results: make(map[string]*Result),
	}

	for wr := range resultCh {
		parallelResult.Results[wr.id] = wr.result
		if wr.err != nil || (wr.result != nil && !wr.result.Success) {
			parallelResult.Failed = append(parallelResult.Failed, wr.id)
		} else {
			parallelResult.Completed = append(parallelResult.Completed, wr.id)
		}
	}

	parallelResult.Duration = time.Since(start)
	return parallelResult, nil
}

type workerResult struct {
	id     string
	result *Result
	err    error
}

func (pe *ParallelExecutor) worker(ctx context.Context, wg *sync.WaitGroup, workCh <-chan SpawnRequest, resultCh chan<- workerResult) {
	defer wg.Done()

	for req := range workCh {
		select {
		case <-ctx.Done():
			resultCh <- workerResult{
				id:  req.Name,
				err: ctx.Err(),
			}
			continue
		default:
		}

		agent, err := pe.manager.SpawnAndWait(ctx, req)
		if err != nil {
			resultCh <- workerResult{
				id:  req.Name,
				err: err,
			}
			continue
		}

		resultCh <- workerResult{
			id:     agent.ID,
			result: agent.Result,
		}
	}
}

// ExecuteWithDependencies runs agents respecting dependencies.
func (pe *ParallelExecutor) ExecuteWithDependencies(ctx context.Context, dag *DependencyGraph) (*ParallelResult, error) {
	start := time.Now()

	parallelResult := &ParallelResult{
		Results: make(map[string]*Result),
	}

	completed := make(map[string]bool)
	var mu sync.Mutex

	for {
		// Find ready nodes (all dependencies satisfied)
		ready := dag.GetReady(completed)
		if len(ready) == 0 {
			break
		}

		// Execute ready nodes in parallel
		var wg sync.WaitGroup
		for _, req := range ready {
			wg.Add(1)
			go func(r SpawnRequest) {
				defer wg.Done()

				agent, err := pe.manager.SpawnAndWait(ctx, r)

				mu.Lock()
				defer mu.Unlock()

				if err != nil || (agent.Result != nil && !agent.Result.Success) {
					parallelResult.Failed = append(parallelResult.Failed, r.Name)
					if agent != nil && agent.Result != nil {
						parallelResult.Results[agent.ID] = agent.Result
					}
				} else {
					completed[r.Name] = true
					parallelResult.Completed = append(parallelResult.Completed, agent.ID)
					parallelResult.Results[agent.ID] = agent.Result
				}
			}(req)
		}

		wg.Wait()

		// Check if we made progress
		if len(parallelResult.Failed) > 0 {
			// Stop on failure
			break
		}
	}

	parallelResult.Duration = time.Since(start)
	return parallelResult, nil
}

// DependencyGraph represents task dependencies.
type DependencyGraph struct {
	nodes        map[string]SpawnRequest
	dependencies map[string][]string // node -> list of dependencies
}

// NewDependencyGraph creates a new dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes:        make(map[string]SpawnRequest),
		dependencies: make(map[string][]string),
	}
}

// AddNode adds a node to the graph.
func (g *DependencyGraph) AddNode(req SpawnRequest, dependsOn ...string) {
	g.nodes[req.Name] = req
	g.dependencies[req.Name] = dependsOn
}

// GetReady returns nodes whose dependencies are all satisfied.
func (g *DependencyGraph) GetReady(completed map[string]bool) []SpawnRequest {
	var ready []SpawnRequest

	for name, req := range g.nodes {
		// Skip if already completed
		if completed[name] {
			continue
		}

		// Check if all dependencies are satisfied
		deps := g.dependencies[name]
		allSatisfied := true
		for _, dep := range deps {
			if !completed[dep] {
				allSatisfied = false
				break
			}
		}

		if allSatisfied {
			ready = append(ready, req)
		}
	}

	return ready
}

// Validate checks for cycles and missing dependencies.
func (g *DependencyGraph) Validate() error {
	// Check for missing dependencies
	for name, deps := range g.dependencies {
		for _, dep := range deps {
			if _, ok := g.nodes[dep]; !ok {
				return fmt.Errorf("node %s depends on missing node %s", name, dep)
			}
		}
	}

	// Check for cycles using DFS
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		visited[node] = true
		recStack[node] = true

		for _, dep := range g.dependencies[node] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	for name := range g.nodes {
		if !visited[name] {
			if hasCycle(name) {
				return fmt.Errorf("cycle detected in dependency graph")
			}
		}
	}

	return nil
}

// Result Aggregators

// DefaultAggregator concatenates all outputs.
type DefaultAggregator struct{}

func (a *DefaultAggregator) Aggregate(results map[string]*Result) (*Result, error) {
	combined := &Result{
		Success: true,
	}

	var outputs []string
	for id, r := range results {
		if r == nil {
			continue
		}

		if !r.Success {
			combined.Success = false
			if combined.Error == "" {
				combined.Error = fmt.Sprintf("agent %s failed: %s", id, r.Error)
			}
		}

		outputs = append(outputs, fmt.Sprintf("=== %s ===\n%s", id, r.Output))
		combined.TokensUsed += r.TokensUsed
		combined.ToolCalls += r.ToolCalls
		combined.Artifacts = append(combined.Artifacts, r.Artifacts...)
	}

	combined.Output = fmt.Sprintf("%d agents completed.\n\n%s",
		len(results), concatStrings(outputs, "\n\n"))

	return combined, nil
}

// FirstSuccessAggregator returns the first successful result.
type FirstSuccessAggregator struct{}

func (a *FirstSuccessAggregator) Aggregate(results map[string]*Result) (*Result, error) {
	for _, r := range results {
		if r != nil && r.Success {
			return r, nil
		}
	}

	return &Result{
		Success: false,
		Error:   "no successful results",
	}, nil
}

// MergeAggregator merges all results using a custom function.
type MergeAggregator struct {
	MergeFunc func(results []*Result) (*Result, error)
}

func (a *MergeAggregator) Aggregate(results map[string]*Result) (*Result, error) {
	var resultSlice []*Result
	for _, r := range results {
		if r != nil {
			resultSlice = append(resultSlice, r)
		}
	}
	return a.MergeFunc(resultSlice)
}

func concatStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
