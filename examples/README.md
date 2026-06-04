# Examples

Go ports of the representative `unibase-aip-sdk` Python examples. Each lives in
its own `package main` directory and is run with `go run ./examples/<name>`.

| Example | Mirrors (Python) | What it shows |
| --- | --- | --- |
| [`server`](server) | — | Minimal A2A echo agent (no platform registration). |
| [`client`](client) | — | Direct A2A client: discover an agent card and `SendTask`. |
| [`platform_client`](platform_client) | `client_example.py` | Calling agents through the AIP platform by handle, auto-routing, and event streaming via `platform.Client`. |
| [`public_agent`](public_agent) | `public_agent_full.py` | Weather agent in Gateway **DIRECT** mode (public endpoint, registered with the platform). |
| [`private_agent`](private_agent) | `private_agent_full.py` | Calculator agent in Gateway **POLLING** mode (behind firewall/NAT, polls the Gateway). |
| [`streaming_agent`](streaming_agent) | `streaming_agent.py` | Streaming agent over SSE (mock token stream instead of OpenAI). |
| [`prediction_market_agent`](prediction_market_agent) | `prediction_market_agent.py` | Marketplace agent: auto-register + job-queue polling + JWT auth + a job offering with input/deliverable schemas. |
| [`binance_agent`](binance_agent) | `agent_sdk_startup_guide.py` | Real Binance price agent with job offerings; four startup modes (auto/manual × push/polling) + JWT auth. |
| [`commerce`](commerce) | `agent_commerce_demo.py` | ERC-8183 job-market flow: create → accept → submit → complete. |
| [`evaluator`](evaluator) | `evaluator_logic.py` | AI evaluator: fetch a submitted job, verify, then complete or reject. |
| [`auto_verification`](auto_verification) | `auto_verification_demo.py` | `SchemaEvaluator` flow: create with schemas → submit → auto-validate → settle. |
| [`auto_submit_agent`](auto_submit_agent) | `automatic_submit_agent.py` | Agent that submits its deliverable to the commerce layer when a job triggered the task. |

## Running

The agent/server examples bind a local port and serve until you press Ctrl+C:

```sh
go run ./examples/server          # echo agent on :8000
go run ./examples/client          # call the echo agent
go run ./examples/streaming_agent # SSE on :8000 (POST /a2a/stream)
```

The platform/registration/commerce examples talk to a running AIP platform and
Gateway. Point them at your deployment with environment variables:

```sh
export AIP_ENDPOINT="http://localhost:8001"
export GATEWAY_URL="http://localhost:8080"
export MEMBASE_ACCOUNT="0x5ea13664c5ce67753f208540d25b913788aa3daa"  # shared test account

go run ./examples/platform_client
go run ./examples/public_agent
go run ./examples/private_agent
go run ./examples/commerce
go run ./examples/auto_verification
go run ./examples/evaluator <job_id>
go run ./examples/auto_submit_agent

# Binance agent supports four startup modes (default: auto):
go run ./examples/binance_agent                 # auto-register + PUSH
go run ./examples/binance_agent manual          # manual register + PUSH
go run ./examples/binance_agent polling         # auto-register + POLLING
go run ./examples/binance_agent polling-manual  # manual register + POLLING
```

## Environment variables

| Variable | Used by | Default |
| --- | --- | --- |
| `AIP_ENDPOINT` | platform_client, public/private agent | `http://localhost:8001` |
| `AIP_PLATFORM_URL` | commerce | `http://localhost:8000` |
| `GATEWAY_URL` | public/private agent | `http://localhost:8080` |
| `MEMBASE_ACCOUNT` | public/private agent | shared test wallet |
| `AGENT_HOST` / `AGENT_PORT` | public/private/prediction agent | `0.0.0.0` / `8200`,`8201` |
| `AGENT_PUBLIC_URL` | public_agent | `http://your-public-ip:<port>` |
| `UNIBASE_PROXY_AUTH` | prediction_market_agent | — (else interactive auth) |
| `UNIBASE_PAY_URL` | prediction_market_agent | `https://api.pay.unibase.com` |

## Notes on porting

- **DIRECT vs POLLING** is selected by `ExposeOptions.EndpointURL`: a non-empty
  value means the Gateway calls the agent directly; an empty value triggers
  gateway polling (the server polls for tasks).
- The streaming example emits a **mock token stream** because Go has no OpenAI/
  ag-ui dependency in this SDK; the streaming mechanics (channel → SSE) are real.
- The commerce example uses `commerce.JobClient`. The Python demo's
  `MissionClient`/`mission_id` map to the job-market API (`job_id`); the example
  accepts either key in the response.
- `prediction_market_agent` keeps the Python flow intact — JWT auth (env or
  `~/.unibase/aip-config.json`, else an interactive `/v1/init` flow), wallet from
  the JWT `sub` claim, and a job offering with input/deliverable schemas — but
  replaces the OpenAI call with a deterministic offline estimate so it builds and
  runs without an API key.
