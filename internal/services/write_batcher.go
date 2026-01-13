package services

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// WriteBatcherConfig configures the write batcher behavior
type WriteBatcherConfig struct {
	FlushInterval time.Duration // How often to flush (default: 30s)
	MaxBatchSize  int           // Max items before forced flush (default: 100)
	Name          string        // Name for logging (e.g., "audit_logs")
}

// DefaultWriteBatcherConfig returns sensible defaults
func DefaultWriteBatcherConfig(name string) WriteBatcherConfig {
	return WriteBatcherConfig{
		FlushInterval: 30 * time.Second,
		MaxBatchSize:  100,
		Name:          name,
	}
}

// WriteBatcher buffers writes and flushes them periodically or when threshold is reached.
// This reduces write contention on SQLite by batching multiple writes into single transactions.
type WriteBatcher[T any] struct {
	config  WriteBatcherConfig
	flushFn func([]T) error // Function to flush items to database

	mu     sync.Mutex
	buffer []T

	// Lifecycle
	flushTicker *time.Ticker
	stopCh      chan struct{}
	wg          sync.WaitGroup

	// Stats
	itemsBuffered int64
	itemsFlushed  int64
	flushCount    int64
	flushErrors   int64
}

// NewWriteBatcher creates a new write batcher with the given flush function.
// The flushFn should perform a batch INSERT of all provided items.
func NewWriteBatcher[T any](config WriteBatcherConfig, flushFn func([]T) error) *WriteBatcher[T] {
	return &WriteBatcher[T]{
		config:  config,
		flushFn: flushFn,
		buffer:  make([]T, 0, config.MaxBatchSize),
		stopCh:  make(chan struct{}),
	}
}

// Start begins the periodic flush goroutine
func (wb *WriteBatcher[T]) Start() {
	wb.flushTicker = time.NewTicker(wb.config.FlushInterval)

	wb.wg.Add(1)
	go func() {
		defer wb.wg.Done()
		for {
			select {
			case <-wb.flushTicker.C:
				if err := wb.Flush(); err != nil {
					slog.Error("write batcher flush failed",
						"name", wb.config.Name,
						"error", err,
					)
				}
			case <-wb.stopCh:
				return
			}
		}
	}()

	slog.Info("write batcher started",
		"name", wb.config.Name,
		"flush_interval", wb.config.FlushInterval,
		"max_batch_size", wb.config.MaxBatchSize,
	)
}

// Stop gracefully stops the batcher, flushing any remaining items
func (wb *WriteBatcher[T]) Stop() {
	close(wb.stopCh)
	if wb.flushTicker != nil {
		wb.flushTicker.Stop()
	}
	wb.wg.Wait()

	// Final flush of remaining items
	if err := wb.Flush(); err != nil {
		slog.Error("write batcher final flush failed",
			"name", wb.config.Name,
			"error", err,
		)
	}

	slog.Info("write batcher stopped",
		"name", wb.config.Name,
		"total_items_buffered", atomic.LoadInt64(&wb.itemsBuffered),
		"total_items_flushed", atomic.LoadInt64(&wb.itemsFlushed),
		"total_flushes", atomic.LoadInt64(&wb.flushCount),
		"flush_errors", atomic.LoadInt64(&wb.flushErrors),
	)
}

// Add queues an item for batched writing.
// If the buffer reaches MaxBatchSize, it triggers an immediate flush.
func (wb *WriteBatcher[T]) Add(item T) {
	wb.mu.Lock()
	wb.buffer = append(wb.buffer, item)
	bufferLen := len(wb.buffer)
	wb.mu.Unlock()

	atomic.AddInt64(&wb.itemsBuffered, 1)

	// Trigger immediate flush if buffer is full
	if bufferLen >= wb.config.MaxBatchSize {
		go func() {
			if err := wb.Flush(); err != nil {
				slog.Error("write batcher threshold flush failed",
					"name", wb.config.Name,
					"error", err,
				)
			}
		}()
	}
}

// Flush writes all buffered items to the database
func (wb *WriteBatcher[T]) Flush() error {
	wb.mu.Lock()
	if len(wb.buffer) == 0 {
		wb.mu.Unlock()
		return nil
	}

	// Swap buffer to release lock quickly
	items := wb.buffer
	wb.buffer = make([]T, 0, wb.config.MaxBatchSize)
	wb.mu.Unlock()

	// Perform the actual flush
	err := wb.flushFn(items)
	if err != nil {
		atomic.AddInt64(&wb.flushErrors, 1)
		// Put items back in buffer for retry on next flush
		wb.mu.Lock()
		wb.buffer = append(items, wb.buffer...)
		wb.mu.Unlock()
		return err
	}

	atomic.AddInt64(&wb.itemsFlushed, int64(len(items)))
	atomic.AddInt64(&wb.flushCount, 1)

	slog.Debug("write batcher flushed",
		"name", wb.config.Name,
		"items", len(items),
	)

	return nil
}

// Stats returns current batcher statistics
func (wb *WriteBatcher[T]) Stats() WriteBatcherStats {
	wb.mu.Lock()
	pending := len(wb.buffer)
	wb.mu.Unlock()

	return WriteBatcherStats{
		Name:          wb.config.Name,
		Pending:       pending,
		ItemsBuffered: atomic.LoadInt64(&wb.itemsBuffered),
		ItemsFlushed:  atomic.LoadInt64(&wb.itemsFlushed),
		FlushCount:    atomic.LoadInt64(&wb.flushCount),
		FlushErrors:   atomic.LoadInt64(&wb.flushErrors),
	}
}

// WriteBatcherStats contains statistics about batcher performance
type WriteBatcherStats struct {
	Name          string
	Pending       int
	ItemsBuffered int64
	ItemsFlushed  int64
	FlushCount    int64
	FlushErrors   int64
}
