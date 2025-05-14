# Event-Driven Architecture in GAPI: Draft 1.1

## 1. What Is an Event-Driven System?
At its core, an event-driven system is a model where **components react to messages**—called events—instead of being directly invoked. These events represent facts that something has occurred: a process has started, a message arrived, a resource changed state.

Instead of one function calling another, components **emit events** into a shared environment, and other components **subscribe to and respond** to them. This design decouples the *source* of an action from its *consequences*, enabling more flexible, modular, and reactive systems.

An event-driven system differs from a traditional request-response system in that:

- The original sender may not know or care who responds.
- Messages are typically **asynchronous**.
- Execution flow emerges from the system as a whole, rather than being hardwired in a central controller.

For example, in a basic orchestration task:
- One agent starts and emits a `status.ready` event.
- Another component receives this and proceeds with its own initialization.
- A third agent logs the state change and updates metrics.

No component directly controls another, yet the system behaves in a coordinated, predictable way.

## 2. Core Principles
- **Producers and Consumers**: Events are emitted by *producers* and consumed by *consumers*. They operate independently.
- **Event Streams and Topics**: Events are organized by topic and stream for easy routing and subscription.
- **Temporal Decoupling**: Event timing between producers and consumers doesn’t need to align.
- **Asynchronous Logic**: Events are handled out-of-band and often concurrently.
- **Observability and Introspection**: Events provide a natural audit trail and introspection model.

## 3. Why Use Event-Driven Architecture?
- **Modularity and Reuse**: Components are decoupled and interact only through events, making them easier to develop, reuse, and replace independently.
- **Scalability Through Loose Coupling**: Systems can grow horizontally as event producers and consumers scale separately without tight integration.
- **Introspection and Traceability**: Structured events leave an audit trail that makes system behavior easier to observe, debug, and analyze.
- **Asynchronous Coordination**: Actions are triggered by events rather than direct calls, enabling non-blocking workflows and distributed logic.
- **Resilience and Recovery**: Events can be buffered, replayed, or retried, supporting fault tolerance and graceful error handling.

## 4. Common Pitfalls
- **Loss of Sequence Clarity**: Without careful ordering or timestamping, it can be difficult to determine the exact sequence of operations across components.
- **Debugging Across Boundaries**: Tracing failures across decoupled services can be challenging without centralized logs or observability tools.
- **Contract Drift**: If producers and consumers aren’t aligned on event structure, mismatches can silently break functionality.
- **Event Storms and Fan-Out**: A single event triggering multiple listeners can lead to unmanageable cascades if not rate-limited or batched.
- **Overengineering**: Applying event-driven patterns where simplicity would suffice can introduce unnecessary complexity and abstraction.

## 5. Events in GAPI
GAPI’s architecture is fundamentally event-driven. Events are structured and transported over QUIC using a Protobuf-based envelope containing:

- `stream`: Category (e.g., runtime, audit)
- `module`: Emitting agent name
- `topic`: The specific event type
- `payload`: Protobuf-encoded content

Every component interacts via events—there are no direct calls, and no implicit state sharing.

## 6. Lifecycle Events
Lifecycle functions supported by agents:
- `initialize`, `start`, `stop`, `restart`, `reload`
- Agents must emit `status.ready` when operational
- Optional `preStart`, `postStop` hooks may be defined

## 7. Status and Audit Events
- **Status Events**: `status.ready`, `status.health`, `status.heartbeat`, `status.metrics`
- **Audit Events**: Immutable logs of actions, often separated into their own file/stream

## 8. Log Stream Separation
- Log streams: `runtime`, `audit`, `metrics`, `status`
- Log splitter agents can route logs to different outputs based on stream or level

## 9. Agents as Event Participants
- All agents communicate through events
- ADKs (Go, Python, Shell) provide tools for structured agent behavior
- `describe` for static capabilities, `event` for runtime behavior

## 10. Closing Thoughts
GAPI reflects an event-driven mindset built on clarity, introspection, and composability. Every agent is autonomous, yet observable. Every message is intentional. This is a system that scales not just in size—but in understanding.
