package server

import (
	"encoding/json"
	"sync"
)

// eventBuffer is the queue depth per listener. A listener that falls this far
// behind is dropping frames it could never have rendered anyway, so the write
// is skipped rather than blocking the publisher.
const eventBuffer = 16

// Hub fans server-sent events out to everyone watching a play session. Topics
// are play slugs.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]map[*listener]struct{}
	closed bool
}

type listener struct {
	events chan []byte
	closed bool
}

func NewHub() *Hub {
	return &Hub{topics: make(map[string]map[*listener]struct{})}
}

// Subscribe registers a listener and reports how many are now on the topic.
// A zero count means the hub is shutting down and the caller should stop.
func (h *Hub) Subscribe(topic string) (*listener, int) {
	l := &listener{events: make(chan []byte, eventBuffer)}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(l.events)
		l.closed = true
		return l, 0
	}
	set, ok := h.topics[topic]
	if !ok {
		set = make(map[*listener]struct{})
		h.topics[topic] = set
	}
	set[l] = struct{}{}
	return l, len(set)
}

// Unsubscribe removes a listener and reports how many remain.
func (h *Hub) Unsubscribe(topic string, l *listener) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	set := h.topics[topic]
	if _, ok := set[l]; ok {
		delete(set, l)
		if !l.closed {
			close(l.events)
			l.closed = true
		}
	}
	if len(set) == 0 {
		delete(h.topics, topic)
	}
	return len(set)
}

// Publish encodes one SSE frame and hands it to every listener on the topic.
func (h *Hub) Publish(topic, event string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}

	frame := make([]byte, 0, len(payload)+len(event)+16)
	frame = append(frame, "event: "...)
	frame = append(frame, event...)
	frame = append(frame, "\ndata: "...)
	frame = append(frame, payload...)
	frame = append(frame, '\n', '\n')

	h.mu.RLock()
	defer h.mu.RUnlock()
	for l := range h.topics[topic] {
		select {
		case l.events <- frame:
		default:
		}
	}
}

// Listeners reports how many browsers are watching a topic.
func (h *Hub) Listeners(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.topics[topic])
}

// Close releases every listener so in-flight SSE handlers return and the HTTP
// server can shut down instead of waiting for streams that never end.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for topic, set := range h.topics {
		for l := range set {
			if !l.closed {
				close(l.events)
				l.closed = true
			}
		}
		delete(h.topics, topic)
	}
}
