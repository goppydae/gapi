# Determinism Checklist

Applies to: scheduling, lifecycle, reconciliation, event emission, hashing.

## Inputs

- [ ] all time is injected (clock interface), not `time.Now()` in core logic
- [ ] randomness is injected/seeded and recorded
- [ ] environment variables are read at boundaries and converted into config

## Ordering

- [ ] map iteration is never used for observable ordering
- [ ] sets are serialized in sorted order
- [ ] filesystem traversal is sorted before use

## Concurrency

- [ ] no data races (race detector clean)
- [ ] goroutine scheduling does not affect externally visible ordering
- [ ] channels used with explicit ordering rules (or stable buffering)

## Serialization / Hashing

- [ ] schema_hash uses canonical encoding
- [ ] JSON output is canonicalized if used as an observable
- [ ] protobuf fields that affect hashes are stable and documented

## Tests

- [ ] replay test exists for the feature (same inputs ⇒ same outputs hash)
- [ ] deterministic "golden trace" comparison updated if needed
