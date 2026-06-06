# Cross-language wire contract

The JSON files in [`fixtures/`](fixtures) are the **single source of truth** for
the wire format shared between the Go SDK (`aip-go-sdk`) and the Python SDK
(`unibase-aip-sdk`). Both SDKs must serialize to — and deserialize from — these
exact shapes, or cross-language calls (registration, A2A messages, gateway jobs)
will silently break.

## Fixtures

| Fixture | Produced by (Go) | Produced by (Python) |
| --- | --- | --- |
| `agent_registration.json` | `AgentConfig.ToRegistrationMap()` | `AgentConfig.to_registration_dict()` |
| `agent_card.json` | `AgentConfig.ToAgentCard(...)` | `AgentConfig.to_agent_card().model_dump(by_alias=True)` |
| `job_offering.json` | `types.AgentJobOffering` | `AgentJobOffering.model_dump(by_alias=True)` |
| `aip_metadata.json` | `messaging.AIPMetadata.ToMap()` | `AIPMetadata.to_dict()` |
| `a2a_message.json` | `a2a.Message` (a2a-go) | `a2a.types.Message.model_dump()` |

## How it's enforced

- **Go:** [`contract_test.go`](contract_test.go) builds the canonical values and
  asserts each serializes to JSON semantically equal to its fixture
  (`go test ./contracts/`). Regenerate after an intentional change with
  `UPDATE_FIXTURES=1 go test ./contracts/`.
- **Python (recommended):** ship a mirror test that loads each fixture, validates
  it into the corresponding Pydantic model, and round-trips it back to the same
  JSON. Keep the fixture files identical in both repos (copy, submodule, or a
  shared `contracts` package).

## Invariants worth calling out

- A2A messages use **camelCase** (`messageId`, `contextId`, `taskId`) and a
  `kind` discriminator on the message and each part (`text`/`data`/`file`);
  `role` is `"user"`/`"agent"` and task state is `"completed"` etc. (the A2A
  spec / `a2a-go` v0.3.x JSON line — NOT the proto `ROLE_USER` form).
- AIP metadata (the `_aip` message-metadata block) uses **snake_case**
  (`run_id`, `caller_id`, `payment_authorized`, …).
- The registration body mixes snake_case top-level keys with the camelCase
  `jobOfferings` / `jobResources` arrays — this asymmetry is intentional and must
  match on both sides.
