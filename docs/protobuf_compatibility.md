# Protobuf Compatibility Policy

Governs every `.proto` file in GoPPydae.

## Goals

- Preserve wire compatibility across minor and patch releases.
- Make safe evolution routine, and breaking changes explicit and rare.

## The gate

`mage proto` runs `buf generate`, then `buf lint`, then `buf breaking`
against HEAD. `buf.yaml` configures breaking detection with the `FILE`
category.

Know what that does **not** catch: **a field rename at a stable field
number is not reported.** `FILE` compares wire-level shape - numbers,
types, cardinality - and a rename changes none of those. Renaming
`instance_uuid` to `subject_uuid` at field 1 passes the gate cleanly
while breaking every consumer that referenced the generated accessor by
name.

This is not hypothetical; the repo performed exactly that rename. So:

- A rename is **source-breaking** even when it is wire-compatible.
- Classify it as breaking in review, and say so in the commit.
- The gate cannot enforce this. A reviewer must.

If a rename must happen, treat it as a rename of the concept: the number
is unchanged so there is nothing to reserve, bump the appropriate
version, and update every consumer in the same change. In the silo that
means both code repos, since the generated types cross the kernel and
orchestrator boundary.

## Never (breaking)

- Renumber an existing field.
- Change a field's type incompatibly, for example `int32` to `string`.
- Change `repeated` to singular, or the reverse.
- Change a package or fully-qualified message name without a migration
  plan.
- Reuse a field number that has ever been released.

## Allowed (non-breaking)

- Add fields with new numbers.
- Add enum values, with the caution below.
- Deprecate rather than delete, and reserve the number and the name.
- Add services and methods; old clients keep working.

## Enums

- Never reorder existing values. The numbers are what travel.
- Reserve removed values and their numbers.
- Value `0` is `UNSPECIFIED` or a safe default, always.

## Presence and defaults

- Do not rely on proto3 implicit presence unless the field is declared
  `optional`.
- "Missing" is meaningful only where presence is defined.

## Reservations

When removing a field:

- Mark it deprecated first.
- Reserve its number **and** its name.
- Keep the reservation for the life of the major version, at minimum.

## Versioning

- A wire-breaking change requires a major version bump, or a parallel
  message or service.
- Migrations support dual-read and dual-write where feasible.

## Review checklist

- Classification: breaking, source-breaking, or neither. Renames are
  source-breaking and the tooling will not tell you.
- Migration plan, if breaking.
- Schema hash implications, if relevant.
- Both repos updated together when the change crosses the kernel and
  orchestrator boundary.
