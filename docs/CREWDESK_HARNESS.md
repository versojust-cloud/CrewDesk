# CrewDesk Harness Architecture

## 1. Product boundary

CrewDesk is a general multi-agent execution platform with a virtual office as
one observable client. The office visualizes agents, tasks, tool calls, and
artifacts; it does not define how work is executed.

The Harness is the server-side execution kernel. It must support research,
coding, data analysis, document production, media workflows, and future skill
packs without adding another fixed coordinator pipeline for every domain.

## 2. Core model

| Object | Responsibility |
| --- | --- |
| `AgentManifest` | Declares instructions, model policy, capabilities, handoff targets, context policy, permissions, and execution limits. |
| `Thread` | Durable conversation and workspace boundary. |
| `Turn` | One user request and its lifecycle from queued to completed, failed, cancelled, or waiting for input. |
| `Task` | A structured unit of work. Dependencies form a DAG and enable safe parallel execution. |
| `Item` | Append-only execution record for messages, tool calls, results, handoffs, approvals, artifacts, status changes, and errors. |
| `Artifact` | Uniform output model for documents, source trees, datasets, images, decks, videos, and other deliverables. |

The append-only `Item` stream is the source for WebSocket updates, replay,
evaluation, audit history, and office animation. UI-specific event shapes are
projections of these records.

## 3. Execution loop

Every agent activation follows the same bounded loop:

1. Compile context from the task brief, selected thread items, dependencies,
   memory, and available artifacts.
2. Ask the model for a message, tool call, handoff, or completion decision.
3. Validate the action against the manifest, tool schema, permissions, budget,
   and current task state.
4. Request approval when policy requires it.
5. Execute the tool or target agent with cancellation and idempotency keys.
6. Append typed items and update task/turn state transactionally.
7. Continue until completion, input is required, limits are reached, or the
   run is cancelled.

Model output never grants a capability. The server-owned manifest and runtime
tool registry are authoritative.

## 4. Orchestration policies

The runtime selects a policy per turn or subgraph:

| Policy | Use |
| --- | --- |
| `manager` | A coordinator retains control and invokes agents for bounded subtasks. Default for open-ended requests. |
| `sequential` | Steps have strict ordering and explicit outputs. |
| `parallel` | Independent tasks execute concurrently, then join. |
| `graph` | Dependency-driven execution for mixed sequential and parallel work. |
| `handoff` | Control transfers to a specialist with filtered context. |
| `roundtable` | Multiple agents contribute perspectives to one shared decision. |
| `review_loop` | A producer and reviewer iterate under a finite quality rubric and budget. |

Agent-as-tool and handoff remain separate operations. Agent-as-tool returns a
bounded result to the caller; handoff changes the active owner of the task.

## 5. Context and memory

Context is compiled rather than copied wholesale:

- **Turn context:** user request, current plan, active task, recent relevant items.
- **Task brief:** goal, constraints, dependencies, expected output, and budget.
- **Thread memory:** accepted preferences and prior decisions for the current workspace.
- **Long-term memory:** reusable facts or procedures with provenance and retention rules.
- **Artifacts:** referenced by stable ID and loaded only when the receiving agent needs them.

Tool results are summarized for other agents while raw output remains
addressable. This controls token growth and prevents irrelevant execution logs
from contaminating later decisions.

## 6. Tool runtime

Execution capabilities are registered through a `ToolFactory` registry:

```go
type ToolFactory interface {
    Name() string
    Available(context.Context, RuntimeDeps) Availability
    Build(context.Context, ToolScope) (ToolInstance, error)
    Policy() ToolPolicy
}
```

`ToolScope` carries thread, turn, task, agent, artifact store, emitter, and
idempotency data. Availability is exposed to planners and clients so disabled
capabilities are never assigned. Side-effecting tools declare approval and
sandbox requirements in `ToolPolicy`.

The Claw compatibility pack registers its research, code, writing, image,
video, deck, and game tools through this contract. Dynamic role bindings only
change a manifest's effective capability list; construction and availability
checks remain owned by the Harness catalog.

## 7. Reliability and evaluation

The Harness records enough structure to support:

- cancellation, deadlines, retry policy, and resumable turns;
- per-turn token, latency, tool-call, and cost budgets;
- idempotent tool execution and duplicate-event protection;
- approval gates for publishing, file changes, credentials, and external writes;
- trace replay using stored items and redacted tool inputs;
- metrics for task completion, tool selection, argument validity, handoff
  quality, latency, cost, and artifact acceptance;
- golden tasks and regression comparison across prompts, models, and policies.

## 8. Compatibility migration

| Current Claw concept | CrewDesk Harness |
| --- | --- |
| `Role` | `AgentManifest` adapter |
| `Session` | `Thread` plus one or more `Turn` records |
| Fixed task list | Dependency-aware `Task` graph |
| Tool/LLM WebSocket events | Typed append-only `Item` stream |
| Separate figure/deck/video/game fields | Generic `Artifact` records |
| Fixed coordinator phases | Selectable orchestration policy |
| Frontend `WORKERS` constant | Server-provided manifests with visual metadata |

Migration is incremental:

1. Introduce contracts, registry, and Claw role adapters without changing the
   existing runner.
2. Persist threads, turns, tasks, items, and artifacts; dual-emit legacy
   `claw.*` events during the transition.
3. Move tool construction to factories and run the existing workflow through
   the manager/graph policy.
4. Load agents dynamically in the office client and add task trace, approval,
   retry, and cancellation views.
5. Add an A2A gateway that maps external Agent Cards, messages, tasks, and
   artifacts onto the same Harness objects. Local agents continue using direct
   calls; remote interoperability stays at the boundary.

## 9. Current implementation status

The backend now includes:

- a validated agent registry and Claw role-pack adapter;
- `RunScope` propagation for workspace, thread, turn, task, and agent identity;
- a lifecycle journal with monotonic workspace/thread sequences, backed by
  Postgres in deployed environments and memory in local fallback mode;
- a capability-aware `ToolCatalog` and factory contract;
- registered factories for all 13 Claw execution tools, including dependency
  injection, backend availability checks, network gates, and side-effect
  policies;
- a non-blocking recording emitter that preserves token streaming while
  writing message/tool lifecycle boundaries into the Harness journal;
- a new turn identity for every Claw create, continue, and resume operation.
- discovery and replay APIs at `GET /api/v1/crew/agents` and
  `GET /api/v1/crew/threads/{id}/items`; agent discovery now includes each
  tool's live availability and policy without exposing private prompts.

The next backend change will execute the current coordinator flow through a
dependency-aware manager/graph policy, with first-class cancellation, bounded
retries, and resumable turn state.
