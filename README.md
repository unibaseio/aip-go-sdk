# Unibase AIP SDK for Go

A Go port of the [Unibase AIP](https://unibase.io) (Agent Interoperability Protocol)
SDK, modeled on the Python `unibase-aip-sdk`. It provides both a **client SDK**
(call agents, stream events, run jobs, gateway-mediated calls) and an **agent
SDK** (expose Go functions as A2A-compatible agent services, register with the
platform, poll a gateway for work).

## Install

```sh
go get github.com/unibaseio/aip-go-sdk
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

## Worked example: a marketplace agent

[`examples/prediction_market_agent`](examples/prediction_market_agent/main.go) is
the end-to-end reference: a private agent that registers on-chain, publishes a
**job offering** to the marketplace, and gets hired and paid through the gateway.
It exercises every moving part of the agent SDK in one file.

### The business flow

```
 developer wallet (JWT)
        │  1. authorize
        ▼
 ExposeAsA2A(...) ──2. register──▶ AIP platform ──on-chain (ERC-8004)──▶ agent_id
        │                              │
        │                              │ 3. job offerings indexed for discovery
        ▼                              ▼
 local agent service            Butler / marketplace
   (polls gateway)                     │
        ▲                              │ 4. user hires the offering
        │  5. gateway routes the job   │   (vector search over job offerings)
        └──────────── gateway ◀────────┘
        │  6. handler produces the deliverable
        ▼
   deliverable ──7. settle (X402 micropayment)──▶ provider wallet
```

1. **Authorize.** The developer signs in once; the SDK loads a Privy/Unibase JWT
   (from `UNIBASE_PROXY_AUTH`, a cached config file, or an interactive flow). The
   wallet address is the JWT's `sub` claim and becomes the agent's owner/`user_id`.
2. **Register.** `ExposeAsA2A` posts the agent config to `POST /agents/register`
   with the JWT as a Bearer token, which triggers on-chain ERC-8004 registration
   and returns an `agent_id` (e.g. `erc8004:prediction_market_demo`).
3. **Publish offerings.** The agent's `JobOfferings` are stored and indexed so the
   Butler/marketplace can find the agent by capability.
4. **Discover & hire.** The Butler runs a vector search over job offerings; when a
   user's request matches, it hires the offering.
5. **Route.** The gateway delivers the job. Public agents are called directly
   (PUSH); private agents poll the gateway job queue (POLLING, see below).
6. **Handle.** Your handler receives the job input and returns the deliverable.
7. **Settle.** The platform settles the X402 micropayment to the provider wallet.

### Registration & deployment modes

`ExposeAsA2A` both starts the HTTP service and (optionally) registers the agent.
The key knobs:

```go
srv := wrappers.ExposeAsA2A(wrappers.ExposeOptions{
    Name:         "Prediction Market Agent",
    Handle:       "prediction_market_demo", // unique marketplace handle
    UserID:       userID,                   // wallet from the JWT `sub` claim
    PrivyToken:   authToken,                // Bearer token for /agents/register
    AIPEndpoint:  "https://api.aip.unibase.com",
    GatewayURL:   "https://gateway.aip.unibase.com",
    ChainID:      97,                        // 97=BSC testnet, 56=mainnet, 1=ETH
    CostModel:    &types.CostModel{BaseCallFee: &base},
    JobOfferings: jobOfferings,              // see below
    EndpointURL:  "",                        // "" => POLLING; a URL => PUSH
    ViaGateway:   true,                      // discoverable via the gateway job queue
}, handler, nil)
srv.Run(ctx)
```

| Knob | Effect |
| --- | --- |
| `EndpointURL` set | **PUSH** mode — the gateway calls the agent's public URL directly. |
| `EndpointURL` empty | **POLLING** mode — the agent polls the gateway for work (good behind NAT/firewall). |
| `ViaGateway: true` + job offerings | Poll the **job queue** (`/gateway/jobs/poll`) so the Butler can hire the agent. Without it, polling uses the plain **task queue** (`/gateway/tasks/poll`). |
| `DisableAutoRegister: true` | Skip registration on start (register out of band, e.g. via `platform.Client.RegisterAgent`). |

Registration failures are non-fatal: the service still starts and logs a warning,
so you can develop locally without a reachable platform.

### Job offerings

A **job offering** is the marketplace listing that makes an agent hireable. It
declares what the agent does, what it charges, and the JSON schemas for the input
it requires and the deliverable it returns:

```go
jobOfferings := []types.AgentJobOffering{{
    ID:          "yes_no_probability",
    Name:        "yes_no_probability",
    Description: "Estimates YES/NO probabilities for any prediction market topic.",
    Type:        "JOB",
    PriceV2:     map[string]any{"type": "fixed", "amount": 0.0015, "currency": "USDC"},
    JobInput:    "Will BTC break $150k by end of 2026?", // example input
    JobOutput:   "Topic: ...\nYES: <0-100>%\nNO: <0-100>%\nReasoning: ...",
    Requirement: map[string]any{ // schema the hirer must satisfy
        "type": "object", "required": []string{"topic"},
        "properties": map[string]any{"topic": map[string]any{"type": "string"}},
    },
    Deliverable: map[string]any{ // schema the agent promises to return
        "type": "object", "required": []string{"text"},
        "properties": map[string]any{"text": map[string]any{"type": "string"}},
    },
    SLAMinutes: 1,
    Active:     true,
}}
```

- **`Description` drives discovery** — the Butler vector-searches over it, so write
  it for the buyer.
- **`PriceV2`** carries structured pricing (`{type, amount, currency}`); `Price`
  is the legacy flat fee. The agent's `CostModel` is the per-call fee.
- **`Requirement` / `Deliverable`** are JSON-schema objects. The `commerce.SchemaEvaluator`
  can auto-validate a submitted deliverable against the `Deliverable` schema
  before settling (see [`examples/auto_verification`](examples/auto_verification/main.go)).
- **`Active`, `Restricted`, `Hide`, `SLAMinutes`** control listing visibility and
  the promised turnaround.

### The handler & deliverable

The handler receives the job input as text and returns the deliverable. The gateway
may deliver the input as plain text, as a `<offering> where topic is '<topic>'`
string, or as a JSON envelope (`{"topic": ...}` / `{"text": ...}`), so robust
handlers extract the meaningful field, then return the deliverable content:

```go
func handler(ctx context.Context, input string) (string, error) {
    topic := extractTopic(input)        // text / {"topic":...} / "where topic is '...'"
    return formatYesNo(topic), nil      // returned verbatim as the deliverable text
}
```

### Run it

```sh
# Real run: set a JWT (or let the interactive flow fetch one) and point at your platform
export UNIBASE_PROXY_AUTH="eyJ..."
export AIP_ENDPOINT="https://api.aip.unibase.com"
export GATEWAY_URL="https://gateway.aip.unibase.com"
go run ./examples/prediction_market_agent

# Local smoke test: fake a JWT, point at unreachable services (registration warns
# but the service still serves), then call the handler directly:
PAYLOAD=$(printf '{"sub":"user:0xYOURWALLET"}' | base64 | tr '+/' '-_' | tr -d '=')
UNIBASE_PROXY_AUTH="e30.$PAYLOAD.sig" AIP_ENDPOINT=http://127.0.0.1:9 \
  GATEWAY_URL=http://127.0.0.1:9 AGENT_PORT=8201 go run ./examples/prediction_market_agent &
curl -s http://127.0.0.1:8201/.well-known/agent-card.json          # card + jobOfferings
curl -s -X POST http://127.0.0.1:8201/invoke -H 'Content-Type: application/json' \
  -d '{"message":"Will BTC break below $60000?"}'                   # invoke the handler
```

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
