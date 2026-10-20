package collector

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
	"github.com/gorilla/websocket"
)

type Collector struct {
	mu          sync.RWMutex
	store       *Store
	subscribers map[*websocket.Conn]bool
	subMu       sync.RWMutex
	buffer      []protocol.MetricSnapshot
	bufferSize  int
}

func New(store *Store) *Collector {
	return &Collector{
		store:       store,
		subscribers: make(map[*websocket.Conn]bool),
		bufferSize:  1000,
	}
}

func (c *Collector) Ingest(snapshot protocol.MetricSnapshot) {
	c.mu.Lock()
	c.buffer = append(c.buffer, snapshot)
	if len(c.buffer) > c.bufferSize {
		c.buffer = c.buffer[len(c.buffer)-c.bufferSize:]
	}
	c.mu.Unlock()

	if c.store != nil {
		if err := c.store.SaveMetric(snapshot); err != nil {
			log.Printf("collector: failed to save metric: %v", err)
		}
	}

	c.broadcast(snapshot)
}

func (c *Collector) IngestChaosEvent(event protocol.ChaosEvent) {
	msg := map[string]interface{}{
		"type":  "chaos_event",
		"event": event,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	c.subMu.RLock()
	defer c.subMu.RUnlock()

	for conn := range c.subscribers {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("collector: ws write chaos event error: %v", err)
		}
	}
}

func (c *Collector) broadcast(snapshot protocol.MetricSnapshot) {
	msg := map[string]any{
		"type":   "metric",
		"metric": snapshot,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	c.subMu.Lock()
	defer c.subMu.Unlock()

	var dead []*websocket.Conn
	for conn := range c.subscribers {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			dead = append(dead, conn)
		}
	}
	for _, conn := range dead {
		delete(c.subscribers, conn)
		conn.Close()
	}
}

func (c *Collector) Subscribe(conn *websocket.Conn) {
	c.subMu.Lock()
	c.subscribers[conn] = true
	c.subMu.Unlock()
}

func (c *Collector) Unsubscribe(conn *websocket.Conn) {
	c.subMu.Lock()
	delete(c.subscribers, conn)
	c.subMu.Unlock()
}

func (c *Collector) RecentMetrics(runID string, limit int) []protocol.MetricSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if runID == "" {
		if limit > 0 && limit < len(c.buffer) {
			return c.buffer[len(c.buffer)-limit:]
		}
		return c.buffer
	}

	var filtered []protocol.MetricSnapshot
	for _, m := range c.buffer {
		if m.RunID == runID {
			filtered = append(filtered, m)
		}
	}
	if limit > 0 && limit < len(filtered) {
		return filtered[len(filtered)-limit:]
	}
	return filtered
}

func (c *Collector) SubscriberCount() int {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	return len(c.subscribers)
}
