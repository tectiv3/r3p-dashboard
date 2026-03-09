package main

import "sync"

// Broker fans out Updates to multiple SSE clients.
type Broker struct {
	mu      sync.RWMutex
	clients map[chan Update]struct{}
}

func NewBroker() *Broker {
	return &Broker{clients: make(map[chan Update]struct{})}
}

func (b *Broker) Subscribe() chan Update {
	ch := make(chan Update, 64)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) Unsubscribe(ch chan Update) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *Broker) Publish(u Update) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- u:
		default:
		}
	}
}
