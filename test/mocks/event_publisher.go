package mocks

import (
	"context"
	"sync"

	"github.com/verdofanv/golang-be/internal/platform/kafka"
)

// EventPublisher records published events for assertions in unit tests.
type EventPublisher struct {
	mu     sync.Mutex
	Events []kafka.Event
	PubErr error
	onPub  chan struct{}
}

func NewEventPublisher() *EventPublisher {
	return &EventPublisher{}
}

func (m *EventPublisher) Publish(_ context.Context, _ string, event kafka.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.PubErr != nil {
		return m.PubErr
	}
	m.Events = append(m.Events, event)
	if m.onPub != nil {
		select {
		case m.onPub <- struct{}{}:
		default:
		}
	}
	return nil
}

// WaitChannel returns a channel that receives a signal on each publish —
// publishing is async (goroutine) in the service, so tests must wait.
func (m *EventPublisher) WaitChannel() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPub = make(chan struct{}, 10)
	return m.onPub
}

func (m *EventPublisher) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Events)
}

func (m *EventPublisher) Last() (kafka.Event, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Events) == 0 {
		return kafka.Event{}, false
	}
	return m.Events[len(m.Events)-1], true
}
