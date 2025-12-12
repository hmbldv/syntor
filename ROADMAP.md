# SYNTOR Development Roadmap

## Current Focus: Multi-Agent Coordination Enhancement

Transform SYNTOR from static agent definitions to a dynamic, runtime-configurable multi-agent system.

### Completed Work

#### TUI Foundation (Complete)
- [x] Bubbletea-based TUI with persistent input bar
- [x] Slash command autocomplete with arrow navigation
- [x] Interrupt capability (Ctrl+C)
- [x] Activity status with live duration
- [x] Forest green theme matching thehumble.dev
- [x] Non-streaming mode for instant full responses
- [x] Text wrapping in viewport
- [x] GPU acceleration via NVIDIA Container Toolkit

---

## Multi-Agent Coordination Phases

### Phase 1: Agent Manifest System ✓
**Goal**: YAML-based agent definitions with runtime loading and hot-reload.

- [x] `pkg/manifest/types.go` - AgentManifest, AgentSpec, PromptSpec structs
- [x] `pkg/manifest/loader.go` - ManifestStore with fsnotify hot-reload
- [x] `pkg/manifest/validator.go` - Schema validation
- [x] Default agent manifests in `configs/agents/`
  - [x] `coordination.yaml`
  - [x] `documentation.yaml`
  - [x] `git.yaml`
  - [x] `worker.yaml`
  - [x] `code.yaml`
- [x] `configs/project.example.yaml` - Example project context

### Phase 2: Dynamic System Prompt Builder ✓
**Goal**: Build prompts that include available agents, capabilities, health, and project context.

- [x] `pkg/prompt/builder.go` - PromptBuilder with template execution
- [x] `pkg/prompt/context.go` - ContextGatherer for agents, project, memory
- [x] Template helper functions (join, upper, lower, indent, dict, list, etc.)
- [x] Replace static `getSystemPrompt()` in TUI with `buildDynamicPrompt()`
- [x] Project context file support (`.syntor/project.yaml`)

### Phase 3: Coordination Protocol ✓
**Goal**: Structured handoff format the LLM can output and we can parse.

- [x] `pkg/coordination/protocol.go` - HandoffIntent, ExecutionPlan types
- [x] `pkg/coordination/parser.go` - Extract JSON intents from LLM output
- [ ] `pkg/coordination/executor.go` - Execute handoffs (integration pending)
- [ ] Kafka integration for async handoffs

### Phase 4: TUI Enhancements ✓
**Goal**: Mode toggle, plan approval, agent activity visualization.

- [x] `Ctrl+A` - Toggle Auto/Plan mode
- [x] `Ctrl+Y` - Approve pending plan
- [x] `Ctrl+N` - Reject pending plan
- [x] `Ctrl+D` - Toggle detail level
- [x] Agent activity panel (real-time handoffs)
- [x] Plan approval UI with message handlers
- [x] Mode indicator in status bar
- [x] Dynamic help bar (shows plan controls when plan pending)

### Phase 5: Context Management ✓
**Goal**: Rich context flow between agents with memory storage.

- [x] `pkg/context/store.go` - ContextStore interface (Store, Item, SessionContext, AgentInteraction)
- [x] `pkg/context/redis.go` - Redis implementation (RedisStore with session/task storage)
- [x] `pkg/context/propagation.go` - TaskContext for handoffs (Fork, AddResult, BuildPromptContext)
- [ ] Integrate memory into prompt building (integration pending)

### Phase 6: Polish & Testing
- [ ] Integration tests for full workflow
- [ ] Hot-reload testing
- [ ] Documentation
- [ ] Performance optimization

---

## Key Design Decisions

1. **YAML for manifests** - Human-readable, supports multi-line prompt templates
2. **Go text/template** - Standard library, familiar, safe
3. **Redis for context** - Already in use, fast, TTL support
4. **Structured JSON in response** - Parseable while staying in natural LLM output
5. **fsnotify for hot-reload** - Cross-platform file watching
6. **Plan mode as safe default** - Users opt-in to auto mode when confident

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         TUI (model.go)                          │
│  [AUTO/PLAN] Toggle | Plan Approval | Agent Activity Panel      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Prompt Builder (pkg/prompt/)                  │
│  Agent Context | Project Context | Memory Store                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│               Manifest Store (pkg/manifest/)                     │
│  ~/.syntor/agents/*.yaml + .syntor/agents/*.yaml (hot-reload)   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│              Handoff Executor (pkg/coordination/)                │
│  Intent Parser | Plan Executor | Result Aggregator              │
└─────────────────────────────────────────────────────────────────┘
```

---

## Configuration Locations

```
~/.syntor/
  config.yaml              # Global config
  agents/                  # Global agent manifests
    custom-agent.yaml

.syntor/                   # Project directory
  config.yaml              # Project config (extends global)
  agents/                  # Project-specific agents
    project-specialist.yaml
  project.yaml             # Project context (values, goals)
```
