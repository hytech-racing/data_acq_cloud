package background

import (
	"encoding/json"
	"log"
	"sync"
)

// A McapStatusEvent is the payload sent to subscribers of the MCAP upload SSE endpoint.
// A status = "processing" event carries the name of a file that started processing, 
// A status = "completed" event carries the serialized MCAP data for a file that finished processing,
// and a status = "failed" event carries the respective error.
type McapStatusEvent struct {
	Status McapStatus  `json:"status"`
	Name   string      `json:"name,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// McapStatusBroadcaster is a simple pub/sub used to broadcast MCAP upload status updates
// to any number of connected SSE subscribers.
type McapStatusBroadcaster struct {
	mu sync.Mutex
	subscribers map[chan []byte]struct{}
}

// NewMcapStatusBroadcaster creates a new McapStatusBroadcaster with no subscribers.
func NewMcapStatusBroadcaster() *McapStatusBroadcaster {
	return &McapStatusBroadcaster{
		subscribers: make(map[chan []byte]struct{}),
	}
}

// Subscribe registers a new subscriber and returns the channel it should read events from.
// The caller is responsible for calling Unsubscribe with the returned channel.
func (b *McapStatusBroadcaster) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	subscriber := make(chan []byte, 16)
	b.subscribers[subscriber] = struct{}{}
	return subscriber
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *McapStatusBroadcaster) Unsubscribe(subscriber chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.subscribers[subscriber]; ok {
		delete(b.subscribers, subscriber)
		close(subscriber)
	}
}

// Broadcast sends an event to all current subscribers. Events are dropped for subscribers
// that are not reading fast enough so that the file processor is never blocked.
func (b *McapStatusBroadcaster) Broadcast(event McapStatusEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("failed to marshal mcap status event: %v", err)
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for subscriber := range b.subscribers {
		select {
		case subscriber <- payload:
		default:
			log.Printf("dropping mcap status event for a slow subscriber")
		}
	}
}
