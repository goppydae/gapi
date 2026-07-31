# GoPPydae Lexicon

*A living record of terms, phrases, and symbols used within the GoPPydae ecosystem.*

This lexicon documents terminology, metaphors, and internal language used across the project. It is not a technical spec - it is a cultural reference for operators, developers, and Goblin sympathizers alike.

______________________________________________________________________

## A-C

**ADK** *(Agent Development Kit)*\
A language-specific toolkit for building agents. Provides lifecycle hooks, metadata declaration, and IPC integration. Both the Python ADK and the Go ADK are implemented and are held to identical semantics by the cross-ADK suite.

**Agent**\
A unit of software supervised by GAPI. Services run continuously; timers fire on a schedule; socket agents are started on demand by traffic. Agents are self-describing, lifecycle-aware, and serve as the primary executable units in the GoPPydae ecosystem.

**Cron Expression**\
A scheduling format borrowed from Unix cron, now supported in timer agents. Examples: `*/5 * * * *` (every 5 minutes), `@hourly`, `@daily`.

**Crystal**\
The symbolic object held by the Goblin. Represents insight, integrity, and structured truth. Often used metaphorically to refer to clarity in system introspection or logging.

______________________________________________________________________

## D-F

**Daemon**\
The long-running background process of the supervisor. In GoPPydae, conventional daemons are referred to as agents when they comply with lifecycle and introspection interfaces.

**Deviling** *(verb)*\
The act of engaging in active development work - particularly around daemons, agents, or the ecosystem itself. Derived from the traditional `devel/` SVN naming convention, reframed as a Goblin cultural term.

______________________________________________________________________

## G-L

**Goblin**\
The mascot and supervisory entity of GoPPydae. Embodies structured orchestration, event clarity, and quiet precision. Also the name of the multi-node cluster daemon.

**GoPPy**\
Pronounced like *copy*. A coined term for the Go + Protobuf + Python stack used throughout the ecosystem. The foundation of GoPPydae's architectural design.

**GoPPydae**\
The full ecosystem built on the GoPPy stack. Includes Goblin, agents, control tools, and supporting infrastructure. Modeled after the Gobiidae fish family, reflecting adaptability and cluster behavior.

**Lounge**\
A conceptual space for idea exchange, lore-building, and system philosophy. Work does not get implemented in the lounge - but it often gets defined there.

______________________________________________________________________

## M-R

**Mage**\
The build orchestration tool used in GoPPydae. Provides tasks for building, testing, proto generation, and more. Invoked via `mage <task>`.

**Schema**\
The structured metadata format that agents must conform to. Includes validation rules for ID, type, resource limits, schedules, and network addresses. Enforced at discovery time.

______________________________________________________________________

## S-Z

**Seal** *(verb)*\
To cryptographically sign an agent using Ed25519. Sealed agents carry a signature that can be verified against a public key, ensuring code integrity.

**Stream**\
A structured data or message flow. In GoPPydae, streams may refer to IPC channels, event flows, or categorized logging output.

**Supervisor**\
The process responsible for managing agent lifecycles on one machine - `gapid` standalone, or the kernel embedded in `goblind`. Cluster coordination is the orchestrator's job, not the supervisor's.

**Timer Agent**\
An agent type that executes on a schedule rather than running continuously. Supports systemd-style intervals (`OnUnitActiveSec=5s`), one-shots (`OnBootSec=`, `OnStartupSec=`), raw durations, cron expressions (`*/5 * * * *`) and descriptors (`@daily`). Both ADKs are scheduled identically.

**TUI** *(Terminal User Interface)*\
The interactive monitoring interface provided by `gapictl tui`. Built with Bubble Tea, it offers real-time agent status, log streaming, and lifecycle control via keyboard shortcuts.

______________________________________________________________________
