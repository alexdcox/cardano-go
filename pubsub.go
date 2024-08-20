package cardano

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type PubSubQueue[T any] interface {
	On(func(message T)) (cleanup func())
	Broadcast(message T)
	Wait(timeout time.Duration) error
	Close()
}

type subscriber[T any] struct {
	id       int
	messages chan T
	callback func(message T)
}

type queue[T any] struct {
	subscribers      map[int]*subscriber[T]
	nextSubscriberID int
	mu               sync.RWMutex
	activeMessages   int64
	closed           atomic.Bool
	done             chan struct{}
}

func NewQueue[T any]() PubSubQueue[T] {
	return &queue[T]{
		subscribers: make(map[int]*subscriber[T]),
		done:        make(chan struct{}),
	}
}

func (q *queue[T]) On(callback func(message T)) (cleanup func()) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed.Load() {
		return func() {}
	}

	id := q.nextSubscriberID
	q.nextSubscriberID++

	sub := &subscriber[T]{
		id:       id,
		messages: make(chan T, 100), // Buffered channel for each subscriber
		callback: callback,
	}

	q.subscribers[id] = sub

	// Start a goroutine to process messages for this subscriber
	go func() {
		for {
			select {
			case msg, ok := <-sub.messages:
				if !ok {
					return
				}
				sub.callback(msg)
				if atomic.AddInt64(&q.activeMessages, -1) == 0 {
					q.mu.RLock()
					if len(q.subscribers) == 0 {
						close(q.done)
					}
					q.mu.RUnlock()
				}
			case <-q.done:
				return
			}
		}
	}()

	return func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if s, exists := q.subscribers[id]; exists {
			delete(q.subscribers, id)
			close(s.messages)
		}
	}
}

func (q *queue[T]) Broadcast(message T) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed.Load() {
		return
	}

	activeSubscribers := len(q.subscribers)
	if activeSubscribers == 0 {
		return
	}

	atomic.AddInt64(&q.activeMessages, int64(activeSubscribers))

	for _, sub := range q.subscribers {
		select {
		case sub.messages <- message:
		default:
			// If the subscriber's queue is full, we'll process the message immediately
			go func(s *subscriber[T]) {
				s.callback(message)
				if atomic.AddInt64(&q.activeMessages, -1) == 0 {
					q.mu.RLock()
					if len(q.subscribers) == 0 {
						close(q.done)
					}
					q.mu.RUnlock()
				}
			}(sub)
		}
	}
}

func (q *queue[T]) Wait(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-q.done:
		return nil
	case <-timer.C:
		return errors.New("timeout waiting for messages to be processed")
	}
}

func (q *queue[T]) Close() {
	if q.closed.Swap(true) {
		return // Already closed
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for _, sub := range q.subscribers {
		close(sub.messages)
	}
	q.subscribers = make(map[int]*subscriber[T])
	close(q.done)
}
