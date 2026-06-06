package models

import (
	"sync"
	"time"
)

// PromptStatus represents the state of a pending prompt
type PromptStatus string

const (
	StatusPending    PromptStatus = "pending"
	StatusProcessing PromptStatus = "processing"
	StatusCompleted  PromptStatus = "completed"
	StatusTimeout    PromptStatus = "timeout"
)

// PendingPrompt represents a prompt waiting to be processed by Kiro
type PendingPrompt struct {
	ID        string         `json:"id"`
	Messages  []ChatMessage  `json:"messages"`
	Model     string         `json:"model,omitempty"`
	Status    PromptStatus   `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	Response  string         `json:"response,omitempty"`
	Done      chan struct{}   `json:"-"` // Signal when response is ready
}

// PromptQueue manages pending prompts between REST API and MCP Server
type PromptQueue struct {
	mu       sync.RWMutex
	prompts  map[string]*PendingPrompt
	pending  chan string // Channel of prompt IDs waiting to be picked up
	timeout  time.Duration
}

// NewPromptQueue creates a new prompt queue
func NewPromptQueue(timeout time.Duration) *PromptQueue {
	return &PromptQueue{
		prompts: make(map[string]*PendingPrompt),
		pending: make(chan string, 1000),
		timeout: timeout,
	}
}

// Enqueue adds a new prompt to the queue and returns its ID
func (q *PromptQueue) Enqueue(id string, messages []ChatMessage, model string) *PendingPrompt {
	q.mu.Lock()
	defer q.mu.Unlock()

	prompt := &PendingPrompt{
		ID:        id,
		Messages:  messages,
		Model:     model,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		Done:      make(chan struct{}),
	}

	q.prompts[id] = prompt
	q.pending <- id

	return prompt
}

// Dequeue gets the next pending prompt (called by Kiro via MCP)
func (q *PromptQueue) Dequeue() *PendingPrompt {
	select {
	case id := <-q.pending:
		q.mu.Lock()
		defer q.mu.Unlock()

		prompt, exists := q.prompts[id]
		if !exists {
			return nil
		}
		prompt.Status = StatusProcessing
		return prompt
	default:
		return nil
	}
}

// SubmitResponse submits a response for a pending prompt (called by Kiro via MCP)
func (q *PromptQueue) SubmitResponse(id string, response string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	prompt, exists := q.prompts[id]
	if !exists {
		return false
	}

	prompt.Response = response
	prompt.Status = StatusCompleted
	close(prompt.Done) // Signal that response is ready

	return true
}

// Get retrieves a prompt by ID
func (q *PromptQueue) Get(id string) *PendingPrompt {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.prompts[id]
}

// Remove removes a prompt from the queue
func (q *PromptQueue) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.prompts, id)
}

// PendingCount returns the number of prompts waiting to be picked up
func (q *PromptQueue) PendingCount() int {
	return len(q.pending)
}

// TotalCount returns total prompts in the queue (all statuses)
func (q *PromptQueue) TotalCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.prompts)
}

// Timeout returns the configured timeout duration
func (q *PromptQueue) Timeout() time.Duration {
	return q.timeout
}
