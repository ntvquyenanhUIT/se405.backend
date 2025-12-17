package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"iamstagram_22520060/internal/queue"
)

const (
	// DefaultWorkerCount is the default number of worker goroutines
	DefaultWorkerCount = 2

	// DefaultBatchSize is the number of messages to read per batch
	DefaultBatchSize = 10

	// DefaultBlockTimeout is how long to block waiting for new messages
	DefaultBlockTimeout = 5 * time.Second
)

// Manager orchestrates worker goroutines that consume from Redis Streams.
type Manager struct {
	consumer    queue.Consumer
	handler     *Handler
	workerCount int
	batchSize   int64
	blockTime   time.Duration

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// ManagerConfig holds configuration for the worker manager.
type ManagerConfig struct {
	WorkerCount  int           // Number of worker goroutines
	BatchSize    int64         // Messages per read
	BlockTimeout time.Duration // Block time for XREADGROUP
}

// DefaultManagerConfig returns sensible defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		WorkerCount:  DefaultWorkerCount,
		BatchSize:    DefaultBatchSize,
		BlockTimeout: DefaultBlockTimeout,
	}
}

// NewManager creates a new worker manager.
func NewManager(consumer queue.Consumer, handler *Handler, cfg ManagerConfig) *Manager {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = DefaultWorkerCount
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.BlockTimeout <= 0 {
		cfg.BlockTimeout = DefaultBlockTimeout
	}

	return &Manager{
		consumer:    consumer,
		handler:     handler,
		workerCount: cfg.WorkerCount,
		batchSize:   cfg.BatchSize,
		blockTime:   cfg.BlockTimeout,
	}
}

// Start begins the worker goroutines.
// Call Stop() to gracefully shut down.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Ensure consumer group exists
	if err := m.consumer.EnsureGroup(m.ctx, queue.StreamFeed, queue.ConsumerGroupFeed); err != nil {
		return err
	}

	log.Printf("[Manager] Starting %d workers for stream=%s group=%s",
		m.workerCount, queue.StreamFeed, queue.ConsumerGroupFeed)

	// Spin up worker goroutines
	for i := 0; i < m.workerCount; i++ {
		workerID := i + 1
		consumerName := consumerNameForWorker(workerID)

		m.wg.Add(1)
		go m.runWorker(workerID, consumerName)
	}

	log.Printf("[Manager] All %d workers started", m.workerCount)
	return nil
}

// Stop gracefully shuts down all workers.
// Blocks until all workers have finished.
func (m *Manager) Stop() {
	log.Printf("[Manager] Stopping workers...")
	m.cancel()
	m.wg.Wait()
	log.Printf("[Manager] All workers stopped")
}

// runWorker is the main loop for a single worker goroutine.
func (m *Manager) runWorker(workerID int, consumerName string) {
	defer m.wg.Done()

	log.Printf("[Worker-%d] Started (consumer=%s)", workerID, consumerName)

	// First, process any pending messages from previous runs (crash recovery)
	m.processPending(workerID, consumerName)

	// Main loop: read and process new messages
	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[Worker-%d] Shutting down", workerID)
			return
		default:
			m.processMessages(workerID, consumerName)
		}
	}
}

// processPending handles messages that were delivered but not acknowledged.
func (m *Manager) processPending(workerID int, consumerName string) {
	log.Printf("[Worker-%d] Checking for pending messages...", workerID)

	// Type assert to access ReadPending method
	rc, ok := m.consumer.(*queue.RedisConsumer)
	if !ok {
		log.Printf("[Worker-%d] Consumer doesn't support ReadPending", workerID)
		return
	}

	for {
		messages, err := rc.ReadPending(m.ctx, queue.StreamFeed, queue.ConsumerGroupFeed, consumerName, m.batchSize)
		if err != nil {
			log.Printf("[Worker-%d] Error reading pending: %v", workerID, err)
			return
		}

		if len(messages) == 0 {
			log.Printf("[Worker-%d] No pending messages", workerID)
			return
		}

		log.Printf("[Worker-%d] Processing %d pending messages", workerID, len(messages))
		m.handleMessages(workerID, messages)
	}
}

// processMessages reads and handles a batch of messages.
func (m *Manager) processMessages(workerID int, consumerName string) {
	messages, err := m.consumer.Read(
		m.ctx,
		queue.StreamFeed,
		queue.ConsumerGroupFeed,
		consumerName,
		m.batchSize,
		m.blockTime,
	)

	if err != nil {
		log.Printf("[Worker-%d] Error reading: %v", workerID, err)
		time.Sleep(time.Second) // Back off on error
		return
	}

	if len(messages) == 0 {
		return // Timeout, no messages
	}

	log.Printf("[Worker-%d] Received %d messages", workerID, len(messages))
	m.handleMessages(workerID, messages)
}

// handleMessages processes a batch of messages and acknowledges them.
func (m *Manager) handleMessages(workerID int, messages []queue.Message) {
	for _, msg := range messages {
		log.Printf("[Worker-%d] Processing msgID=%s type=%s", workerID, msg.ID, msg.Event.Type)

		err := m.handler.HandleEvent(m.ctx, msg.Event)
		if err != nil {
			log.Printf("[Worker-%d] Handler error msgID=%s: %v", workerID, msg.ID, err)
			// Still ACK to prevent infinite retry loops
			// In production, you might want dead-letter queue here
		}

		// Acknowledge the message
		if err := m.consumer.Ack(m.ctx, queue.StreamFeed, queue.ConsumerGroupFeed, msg.ID); err != nil {
			log.Printf("[Worker-%d] ACK error msgID=%s: %v", workerID, msg.ID, err)
		}
	}
}

// consumerNameForWorker generates a unique consumer name for each worker.
func consumerNameForWorker(workerID int) string {
	return "worker-" + string(rune('0'+workerID))
}
