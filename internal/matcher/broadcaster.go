package matcher

import (
	"sync"

	pb "subnet/proto/subnet"
)

// intentBroadcaster fan-outs intent updates to active subscribers.
type intentBroadcaster struct {
	mu   sync.Mutex
	subs map[uint64]chan *pb.MatcherIntentUpdate
	next uint64
}

func newIntentBroadcaster() *intentBroadcaster {
	return &intentBroadcaster{subs: make(map[uint64]chan *pb.MatcherIntentUpdate)}
}

func (b *intentBroadcaster) subscribe() (uint64, <-chan *pb.MatcherIntentUpdate) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan *pb.MatcherIntentUpdate, 32)
	b.subs[id] = ch
	return id, ch
}

func (b *intentBroadcaster) unsubscribe(id uint64) {
	b.mu.Lock()
	ch, ok := b.subs[id]
	if ok {
		delete(b.subs, id)
	}
	b.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (b *intentBroadcaster) broadcast(update *pb.MatcherIntentUpdate) {
	b.mu.Lock()
	subs := make([]chan *pb.MatcherIntentUpdate, 0, len(b.subs))
	for _, ch := range b.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- update:
		default:
		}
	}
}

// bidBroadcaster delivers bid events.
type bidBroadcaster struct {
	mu   sync.Mutex
	subs map[uint64]chan *pb.BidEvent
	next uint64
}

func newBidBroadcaster() *bidBroadcaster {
	return &bidBroadcaster{subs: make(map[uint64]chan *pb.BidEvent)}
}

func (b *bidBroadcaster) subscribe() (uint64, <-chan *pb.BidEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan *pb.BidEvent, 64)
	b.subs[id] = ch
	return id, ch
}

func (b *bidBroadcaster) unsubscribe(id uint64) {
	b.mu.Lock()
	ch, ok := b.subs[id]
	if ok {
		delete(b.subs, id)
	}
	b.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (b *bidBroadcaster) broadcast(evt *pb.BidEvent) {
	b.mu.Lock()
	subs := make([]chan *pb.BidEvent, 0, len(b.subs))
	for _, ch := range b.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
		}
	}
}
