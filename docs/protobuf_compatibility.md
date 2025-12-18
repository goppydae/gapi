# Protobuf Compatibility Policy

This policy governs all `.proto` files in GoPPydae.

## Goals

- preserve wire compatibility across minor/patch releases
- enable safe evolution of messages and services
- make breaking changes explicit and rare

## Never Do (Breaking)

- renumber an existing field
- change a field's type in an incompatible way (e.g., int32 → string)
- change `repeated` ↔ singular
- change package or fully-qualified message names without a migration plan
- reuse a field number once released

## Allowed (Non-breaking)

- add new fields with new numbers
- add new enum values (with caution; see below)
- deprecate fields (do not delete) and reserve numbers/names
- add new services/methods (old clients still work)

## Enums

- never reorder existing values (numbers are what matter)
- reserve removed values and numbers
- treat `0` as “UNSPECIFIED” or safe default

## Field Presence & Defaults

- don’t rely on proto3 default presence unless explicitly using `optional`
- treat “missing” as meaningful only when presence is defined

## Reservations

When removing a field:

- mark it deprecated
- reserve its field number and name
- keep it reserved forever (or for a defined major version window)

## Versioning Rules

- wire-breaking changes require a major version bump OR a parallel message/service
- migrations must support dual-read / dual-write when feasible

## Required Review Items

- compatibility classification: breaking vs non-breaking
- migration plan (if breaking)
- updated schema hash semantics (if relevant)
