# NATS JetStream Evaluation

## Current State
Using NATS Core for consensus event broadcasting with custom deduplication.

## JetStream Benefits
1. **Message Persistence** - Messages stored to disk, survives restarts
2. **Exactly-Once Delivery** - Built-in deduplication
3. **Message Replay** - Can replay events from specific point
4. **Consumer Groups** - Multiple validators can form consumer groups
5. **Acknowledgments** - Ensures message processing

## Implementation Considerations

### Pros
- Remove custom Deduper implementation
- Better reliability for production
- Built-in message ordering
- Automatic retention policies

### Cons
- Additional complexity for MVP
- Requires NATS server with JetStream enabled
- Higher resource usage (disk storage)
- Learning curve for operators

## Recommendation
**DEFER** - Current NATS Core implementation is sufficient for MVP. Consider JetStream for production when:
- Need message durability across restarts
- Require guaranteed delivery semantics
- Scale beyond 10+ validators

## Migration Path
If needed later:
1. Enable JetStream in NATS config: `jetstream { store_dir: "/data" }`
2. Update NatsBroadcaster to use JetStream APIs
3. Configure streams for each topic:
   - checkpoint.propose
   - checkpoint.signature
   - checkpoint.finalized
4. Update subscribers to use JetStream consumers

## Code Example (Future)
```go
// Create stream
js, _ := nc.JetStream()
js.AddStream(&nats.StreamConfig{
    Name:     "CHECKPOINT",
    Subjects: []string{"checkpoint.>"},
    Storage:  nats.FileStorage,
    Replicas: 3,
})

// Publish with dedup
js.Publish("checkpoint.propose", data, 
    nats.MsgId(msgID), // Dedup key
    nats.ExpectStream("CHECKPOINT"))

// Subscribe with durable consumer
js.Subscribe("checkpoint.propose", handler,
    nats.Durable("validator-v1"),
    nats.AckExplicit())
```