# Unibase AIP SDK for Go

A Go port of the [Unibase AIP](https://unibase.io) (Agent Interoperability Protocol)
SDK, modeled on the Python `unibase-aip-sdk`. It provides both a **client SDK**
(call agents, stream events, run jobs, gateway-mediated calls) and an **agent
SDK** (expose Go functions as A2A-compatible agent services, register with the
platform, poll a gateway for work).

## Install

```sh
go get github.com/unibaseio/unibase-aip-sdk-go
```

Requires Go 1.25+.

## Packages

| Package | Purpose |
| --- | --- |
| `aiperr` | Error types and codes (`AIPError` and friends). |
| `core` | `AgentType`, `AgentIdentity`. |
| `a2a` | A2A protocol types (`Task`, `Message`, `Part`, …) aliased from the official [a2a-go](https://github.com/a2aproject/a2a-go) SDK (v0.3.x), the A2A `Client` (wrapping `a2aclient`), and agent-card generation. |
| `types` | SDK data models: `AgentCard` (ERC-8004), `AgentConfig`, `CostModel`, `RunResult`, `AgentMessage`, etc. |
| `messaging` | AIP metadata embedded in A2A messages plus message helper functions. |
| `agent` | `AIPContext` envelope, message wrap/unwrap, and `ExternalAgentClient` (gateway task puller). |
| `platform` | `Client` for the AIP platform: health, agent/user registration, `Run`/`RunStream`, pricing, runs, and jobs. |
| `gateway` | `Client` (gateway registration) and `A2AClient` (push/pull gateway-mediated calls). |
| `commerce` | `JobClient` and `SchemaEvaluator` for Agentic Commerce. |
| `registry` | `Client` wrapping the platform client for agent management and A2A discovery. |
| `server` | A2A HTTP server (`net/http`) exposing an agent, with auto-registration and gateway polling. |
| `wrappers` | `ExposeAsA2A` — turn a plain Go function into an A2A agent service. |

## Quick start

### Expose a function as an agent

```go
srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
    Name: "Echo Agent",
    Host: "127.0.0.1",
    Port: 8000,
}, func(ctx context.Context, input string) (string, error) {
    return "Echo: " + input, nil
}, nil)

ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT)
defer stop()
srv.Run(ctx)
```

### Call an agent

```go
client := a2a.NewClient(0, nil)
msg := a2a.NewMessage(a2a.RoleUser, uuid.NewString(), "hello")
task, err := client.SendTask(ctx, "http://127.0.0.1:8000", msg, "", "", nil)
fmt.Println(a2a.GetMessageText(&task.History[len(task.History)-1]))
```

### Run a task on the platform

```go
pc := platform.New("") // defaults to $AIP_ENDPOINT or http://localhost:8001
result, _ := pc.Run(ctx, "summarize this document", platform.RunOptions{UserID: "user:0x..."})
fmt.Println(result.Success(), result.Output())
```

See [`examples/`](examples) for runnable `server` and `client` programs.

## Design notes

- **Async → context + channels.** Python coroutines map to `context.Context`
  parameters; async generators (streaming) map to Go channels. The Python SDK's
  separate sync/async clients collapse into one context-aware client per service.
- **camelCase wire format.** A2A protocol JSON uses camelCase field names
  (`messageId`, `contextId`, …) to stay compatible with the reference
  implementation and Google's A2A SDK. ERC-8004 agent cards follow the spec's
  camelCase keys.
- **Official A2A types.** The `a2a` package aliases the protocol types from the
  official `github.com/a2aproject/a2a-go` SDK and backs the outbound client with
  its `a2aclient`. The **v0.3.x** line is used deliberately: it is JSON/spec
  compatible (`role: "user"`, `state: "completed"`, parts carry a `kind`
  discriminator), matching the Google A2A Python ecosystem the Unibase platform
  speaks. The **v2.x** line switched to a proto/gRPC wire format
  (`ROLE_USER`, `TASK_STATE_COMPLETED`, no `kind`) and is **not** interoperable
  with that ecosystem. Pulling in `a2aclient` also brings gRPC/protobuf as
  transitive dependencies.
- **Custom server, official types.** The Unibase A2A server stays a `net/http`
  implementation (it exposes `/invoke`, `/conversations`, gateway job/task
  polling, and auto-registration that `a2asrv` doesn't provide) but produces and
  consumes a2a-go types. It serves `message/stream` on both `/a2a/stream` and the
  main `/a2a` JSON-RPC endpoint so a2a-go's single-URL transport can stream.

## Not ported

The Python SDK ships framework-specific adapters that have no Go equivalent and
are intentionally omitted:

- LangGraph (`expose_langgraph_as_a2a`, `LangGraphWrapper`)
- Google ADK (`expose_adk_as_a2a`, `ADKWrapper`)
- ag-ui / Vercel AI SSE shims and the `/agui/stream` endpoint
- Claude / OpenAI / LangChain LLM adapters
- Membase memory initialization in the registry

The callback-based `AgentContext` from `types.py` is also omitted in favor of
the explicit `agent.AIPContext` envelope and direct client calls.
