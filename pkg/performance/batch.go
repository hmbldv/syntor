package performance

import (
	"context"
	"sync"
	"time"
)

// BatchProcessor processes items in batches for efficiency
type BatchProcessor[T any] struct {
	batchSize     int
	flushInterval time.Duration
	processor     func([]T) error

	buffer    []T
	bufferMu  sync.Mutex
	flushChan chan struct{}
	done      chan struct{}
	wg        sync.WaitGroup
}

// BatchProcessorConfig holds configuration for batch processing
type BatchProcessorConfig[T any] struct {
	BatchSize     int
	FlushInterval time.Duration
	Processor     func([]T) error
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor[T any](config BatchProcessorConfig[T]) *BatchProcessor[T] {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 100 * time.Millisecond
	}

	bp := &BatchProcessor[T]{
		batchSize:     config.BatchSize,
		flushInterval: config.FlushInterval,
		processor:     config.Processor,
		buffer:        make([]T, 0, config.BatchSize),
		flushChan:     make(chan struct{}, 1),
		done:          make(chan struct{}),
	}

	return bp
}

// Start begins the background flush goroutine
func (bp *BatchProcessor[T]) Start(ctx context.Context) {
	bp.wg.Add(1)
	go bp.flushLoop(ctx)
}

// Stop stops the batch processor and flushes remaining items
func (bp *BatchProcessor[T]) Stop() {
	close(bp.done)
	bp.wg.Wait()
	bp.Flush() // Final flush
}

// Add adds an item to the batch
func (bp *BatchProcessor[T]) Add(item T) error {
	bp.bufferMu.Lock()
	bp.buffer = append(bp.buffer, item)
	shouldFlush := len(bp.buffer) >= bp.batchSize
	bp.bufferMu.Unlock()

	if shouldFlush {
		bp.triggerFlush()
	}

	return nil
}

// AddBatch adds multiple items at once
func (bp *BatchProcessor[T]) AddBatch(items []T) error {
	bp.bufferMu.Lock()
	bp.buffer = append(bp.buffer, items...)
	shouldFlush := len(bp.buffer) >= bp.batchSize
	bp.bufferMu.Unlock()

	if shouldFlush {
		bp.triggerFlush()
	}

	return nil
}

// Flush forces a flush of the current buffer
func (bp *BatchProcessor[T]) Flush() error {
	bp.bufferMu.Lock()
	if len(bp.buffer) == 0 {
		bp.bufferMu.Unlock()
		return nil
	}

	batch := bp.buffer
	bp.buffer = make([]T, 0, bp.batchSize)
	bp.bufferMu.Unlock()

	if bp.processor != nil {
		return bp.processor(batch)
	}
	return nil
}

// triggerFlush signals the flush loop
func (bp *BatchProcessor[T]) triggerFlush() {
	select {
	case bp.flushChan <- struct{}{}:
	default:
		// Channel full, flush will happen soon
	}
}

// flushLoop runs in the background to periodically flush
func (bp *BatchProcessor[T]) flushLoop(ctx context.Context) {
	defer bp.wg.Done()

	ticker := time.NewTicker(bp.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-bp.done:
			return
		case <-ticker.C:
			bp.Flush()
		case <-bp.flushChan:
			bp.Flush()
		}
	}
}

// Size returns the current buffer size
func (bp *BatchProcessor[T]) Size() int {
	bp.bufferMu.Lock()
	defer bp.bufferMu.Unlock()
	return len(bp.buffer)
}

// BatchAggregator aggregates results from batch operations
type BatchAggregator[T any, R any] struct {
	items       []T
	batchSize   int
	processor   func([]T) ([]R, error)
	mu          sync.Mutex
}

// NewBatchAggregator creates a new batch aggregator
func NewBatchAggregator[T any, R any](batchSize int, processor func([]T) ([]R, error)) *BatchAggregator[T, R] {
	return &BatchAggregator[T, R]{
		items:     make([]T, 0, batchSize),
		batchSize: batchSize,
		processor: processor,
	}
}

// Add adds an item and returns results if batch is full
func (ba *BatchAggregator[T, R]) Add(item T) ([]R, bool, error) {
	ba.mu.Lock()
	ba.items = append(ba.items, item)

	if len(ba.items) < ba.batchSize {
		ba.mu.Unlock()
		return nil, false, nil
	}

	batch := ba.items
	ba.items = make([]T, 0, ba.batchSize)
	ba.mu.Unlock()

	results, err := ba.processor(batch)
	return results, true, err
}

// Flush processes any remaining items
func (ba *BatchAggregator[T, R]) Flush() ([]R, error) {
	ba.mu.Lock()
	if len(ba.items) == 0 {
		ba.mu.Unlock()
		return nil, nil
	}

	batch := ba.items
	ba.items = make([]T, 0, ba.batchSize)
	ba.mu.Unlock()

	return ba.processor(batch)
}

// PipelineStage represents a processing stage in a pipeline
type PipelineStage[In any, Out any] struct {
	name      string
	processor func(In) (Out, error)
	workers   int
}

// Pipeline processes items through multiple stages
type Pipeline[T any] struct {
	input    chan T
	output   chan T
	stages   []func(T) (T, error)
	workers  int
	wg       sync.WaitGroup
}

// NewPipeline creates a new processing pipeline
func NewPipeline[T any](workers int, bufferSize int) *Pipeline[T] {
	if workers <= 0 {
		workers = 1
	}
	if bufferSize <= 0 {
		bufferSize = 100
	}

	return &Pipeline[T]{
		input:   make(chan T, bufferSize),
		output:  make(chan T, bufferSize),
		stages:  make([]func(T) (T, error), 0),
		workers: workers,
	}
}

// AddStage adds a processing stage
func (p *Pipeline[T]) AddStage(processor func(T) (T, error)) {
	p.stages = append(p.stages, processor)
}

// Start starts the pipeline workers
func (p *Pipeline[T]) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Stop stops the pipeline
func (p *Pipeline[T]) Stop() {
	close(p.input)
	p.wg.Wait()
	close(p.output)
}

// Submit submits an item for processing
func (p *Pipeline[T]) Submit(item T) {
	p.input <- item
}

// Results returns the output channel
func (p *Pipeline[T]) Results() <-chan T {
	return p.output
}

// worker processes items through all stages
func (p *Pipeline[T]) worker(ctx context.Context) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-p.input:
			if !ok {
				return
			}

			result := item
			var err error
			for _, stage := range p.stages {
				result, err = stage(result)
				if err != nil {
					break
				}
			}

			if err == nil {
				select {
				case p.output <- result:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}
