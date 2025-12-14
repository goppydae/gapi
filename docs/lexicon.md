# GoPPydae Lexicon

*A living record of terms, phrases, and symbols used within the GoPPydae ecosystem.*

This lexicon documents terminology, metaphors, and internal language used across the project. It is not a technical spec—it is a cultural reference for operators, developers, and Goblin sympathizers alike.

---

## A–C

**ADK** *(Agent Development Kit)*  
A language-specific toolkit for building agents. Provides lifecycle hooks, metadata declaration, and IPC integration. Current implementations include Python ADK and (planned) Go ADK.

**Agent**  
A long-running process supervised by GAPI. Agents are self-describing, lifecycle-aware, and serve as the primary executable units in the GoPPydae ecosystem.

**Cron Expression**  
A scheduling format borrowed from Unix cron, now supported in timer agents. Examples: `*/5 * * * *` (every 5 minutes), `@hourly`, `@daily`.

**Crystal**  
The symbolic object held by the Goblin. Represents insight, integrity, and structured truth. Often used metaphorically to refer to clarity in system introspection or logging.

---

## D–F

**Daemon**  
The long-running background process of the supervisor. In GoPPydae, conventional daemons are referred to as agents when they comply with lifecycle and introspection interfaces.

**Deviling** *(verb)*  
The act of engaging in active development work—particularly around daemons, agents, or the ecosystem itself. Derived from the traditional `devel/` SVN naming convention, reframed as a Goblin cultural term.

---

## G–L

**Goblin**  
The mascot and supervisory entity of GoPPydae. Embodies structured orchestration, event clarity, and quiet precision. Also the name of the multi-node cluster daemon.

**Goppy**  
Pronounced like *copy*. A coined term for the Go + Protobuf + Python stack used throughout the ecosystem. The foundation of GoPPydae's architectural design.

**Goppydae**  
The full ecosystem built on the Goppy stack. Includes Goblin, agents, control tools, and supporting infrastructure. Modeled after the Gobiidae fish family, reflecting adaptability and cluster behavior.

**Lounge**  
A conceptual space for idea exchange, lore-building, and system philosophy. Work does not get implemented in the lounge—but it often gets defined there.

---

## M–R

**Mage**  
The build orchestration tool used in GoPPydae. Provides tasks for building, testing, proto generation, and more. Invoked via `mage <task>`.

**Schema**  
The structured metadata format that agents must conform to. Includes validation rules for ID, type, resource limits, schedules, and network addresses. Enforced at discovery time.

---

## S–Z

**Seal** *(verb)*  
To cryptographically sign an agent using Ed25519. Sealed agents carry a signature that can be verified against a public key, ensuring code integrity.

**Stream**  
A structured data or message flow. In GoPPydae, streams may refer to IPC channels, event flows, or categorized logging output.

**Supervisor**  
The process (usually `gapid`) responsible for managing agent lifecycles, receiving events, and coordinating control flow across a node or cluster.

**Timer Agent**  
An agent type that executes on a schedule rather than running continuously. Supports systemd-style intervals (`OnUnitActiveSec=5s`) and cron expressions (`*/5 * * * *`).

**TUI** *(Terminal User Interface)*  
The interactive monitoring interface provided by `gapictl tui`. Built with Bubble Tea, it offers real-time agent status, log streaming, and lifecycle control via keyboard shortcuts.

---
