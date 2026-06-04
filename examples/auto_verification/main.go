// Command auto_verification demonstrates the Virtuals-style automated
// verification flow, mirroring the Python examples/auto_verification_demo.py:
// a client defines a service with required schemas, a provider submits data,
// and a SchemaEvaluator automatically validates and settles.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/unibaseio/aip-go-sdk/commerce"
	"github.com/unibaseio/aip-go-sdk/internal/log"
	"github.com/unibaseio/aip-go-sdk/platform"
)

var logger = log.Get("examples.auto_verification")

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()
	client := platform.New(getenv("AIP_PLATFORM_URL", "http://localhost:8000"))
	market := commerce.NewJobClient(client)

	const evaluatorID = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
	evaluator := commerce.NewSchemaEvaluator(market, evaluatorID)

	// Phase 1: define the service schemas.
	logger.Infof("Defining service schemas...")
	reqSchema := map[string]any{"type": "object", "required": []any{"prompt"}}
	delivSchema := map[string]any{"type": "object", "required": []any{"tx_hash", "status"}}

	// Phase 2: client creates a job with schema enforcement.
	logger.Infof("Client creating job with schema enforcement...")
	job, err := market.Create(ctx, "client_agent_123",
		"Swap 100 USDC for VIRTUAL tokens.", 10.0, "USDC", evaluatorID, 0, 0,
		map[string]any{"requirement_schema": reqSchema, "deliverable_schema": delivSchema})
	if err != nil {
		fmt.Fprintln(os.Stderr, "create failed:", err)
		os.Exit(1)
	}
	jobID, _ := job["job_id"].(string)
	if jobID == "" {
		jobID, _ = job["mission_id"].(string)
	}

	// Phase 3: provider submits valid deliverable data.
	logger.Infof("Provider submitting valid deliverable data...")
	if _, err := market.Submit(ctx, jobID, "provider_agent_456",
		map[string]any{"tx_hash": "0xabc123...", "status": "success"},
		"Swap complete: 0xabc123...", 0); err != nil {
		fmt.Fprintln(os.Stderr, "submit failed:", err)
		os.Exit(1)
	}

	// Phase 4: automated evaluation.
	logger.Infof("Triggering automated SchemaEvaluator...")
	ok, err := evaluator.VerifyAndSettle(ctx, jobID, reqSchema, delivSchema)
	if err != nil {
		fmt.Fprintln(os.Stderr, "evaluate failed:", err)
		os.Exit(1)
	}
	if ok {
		logger.Infof("Demo Result: Automated Settlement Successful!")
	} else {
		logger.Errorf("Demo Result: Settlement Failed.")
	}
}
