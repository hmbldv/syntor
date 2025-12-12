package performance

import (
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/models"
)

// MessagePool provides object pooling for Message objects to reduce allocations
type MessagePool struct {
	pool sync.Pool
}

// NewMessagePool creates a new message pool
func NewMessagePool() *MessagePool {
	return &MessagePool{
		pool: sync.Pool{
			New: func() interface{} {
				return &models.Message{
					Payload: make(map[string]interface{}),
				}
			},
		},
	}
}

// Get retrieves a message from the pool
func (p *MessagePool) Get() *models.Message {
	msg := p.pool.Get().(*models.Message)
	// Reset fields
	msg.ID = ""
	msg.Type = ""
	msg.Source = ""
	msg.Target = ""
	msg.CorrelationID = ""
	msg.ReplyTo = ""
	msg.TTL = 0
	msg.Timestamp = time.Time{}
	// Clear payload map
	for k := range msg.Payload {
		delete(msg.Payload, k)
	}
	return msg
}

// Put returns a message to the pool
func (p *MessagePool) Put(msg *models.Message) {
	if msg == nil {
		return
	}
	p.pool.Put(msg)
}

// TaskPool provides object pooling for Task objects
type TaskPool struct {
	pool sync.Pool
}

// NewTaskPool creates a new task pool
func NewTaskPool() *TaskPool {
	return &TaskPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &models.Task{
					Payload:              make(map[string]interface{}),
					RequiredCapabilities: make([]string, 0, 4),
				}
			},
		},
	}
}

// Get retrieves a task from the pool
func (p *TaskPool) Get() *models.Task {
	task := p.pool.Get().(*models.Task)
	// Reset fields
	task.ID = ""
	task.Type = ""
	task.Priority = 0
	task.AssignedAgent = ""
	task.Status = ""
	task.CreatedAt = time.Time{}
	task.StartedAt = nil
	task.CompletedAt = nil
	task.Result = nil
	task.Error = nil
	task.Timeout = 0
	task.RetryCount = 0
	task.MaxRetries = 0
	// Reset slices
	task.RequiredCapabilities = task.RequiredCapabilities[:0]
	// Clear payload map
	for k := range task.Payload {
		delete(task.Payload, k)
	}
	return task
}

// Put returns a task to the pool
func (p *TaskPool) Put(task *models.Task) {
	if task == nil {
		return
	}
	p.pool.Put(task)
}

// ByteBufferPool provides pooling for byte buffers
type ByteBufferPool struct {
	pool sync.Pool
	size int
}

// NewByteBufferPool creates a new byte buffer pool
func NewByteBufferPool(bufferSize int) *ByteBufferPool {
	return &ByteBufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, bufferSize)
				return &b
			},
		},
		size: bufferSize,
	}
}

// Get retrieves a buffer from the pool
func (p *ByteBufferPool) Get() *[]byte {
	buf := p.pool.Get().(*[]byte)
	*buf = (*buf)[:0] // Reset length but keep capacity
	return buf
}

// Put returns a buffer to the pool
func (p *ByteBufferPool) Put(buf *[]byte) {
	if buf == nil {
		return
	}
	// Only return buffers of expected size
	if cap(*buf) == p.size {
		p.pool.Put(buf)
	}
}

// Global pools
var (
	globalMessagePool    *MessagePool
	globalTaskPool       *TaskPool
	globalBufferPool     *ByteBufferPool
	poolOnce             sync.Once
)

func initPools() {
	globalMessagePool = NewMessagePool()
	globalTaskPool = NewTaskPool()
	globalBufferPool = NewByteBufferPool(4096)
}

// GetMessage gets a message from the global pool
func GetMessage() *models.Message {
	poolOnce.Do(initPools)
	return globalMessagePool.Get()
}

// PutMessage returns a message to the global pool
func PutMessage(msg *models.Message) {
	poolOnce.Do(initPools)
	globalMessagePool.Put(msg)
}

// GetTask gets a task from the global pool
func GetTask() *models.Task {
	poolOnce.Do(initPools)
	return globalTaskPool.Get()
}

// PutTask returns a task to the global pool
func PutTask(task *models.Task) {
	poolOnce.Do(initPools)
	globalTaskPool.Put(task)
}

// GetBuffer gets a buffer from the global pool
func GetBuffer() *[]byte {
	poolOnce.Do(initPools)
	return globalBufferPool.Get()
}

// PutBuffer returns a buffer to the global pool
func PutBuffer(buf *[]byte) {
	poolOnce.Do(initPools)
	globalBufferPool.Put(buf)
}
