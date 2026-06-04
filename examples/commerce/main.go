// Command commerce demonstrates the ERC-8183 job-market flow, mirroring the
// Python examples/agent_commerce_demo.py: a client creates a job, a provider
// accepts and submits a deliverable, and an evaluator completes/settles it.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/unibaseio/aip-go-sdk/commerce"
	"github.com/unibaseio/aip-go-sdk/internal/log"
	"github.com/unibaseio/aip-go-sdk/platform"
)

var logger = log.Get("examples.commerce")

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	aipBaseURL := getenv("AIP_PLATFORM_URL", "http://localhost:8000")
	client := platform.New(aipBaseURL)
	market := commerce.NewJobClient(client)

	const (
		clientAgentID   = "agent_uuid_client_123"
		providerAgentID = "agent_uuid_provider_456"
		evaluatorID     = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
	)

	ctx := context.Background()

	logger.Infof("--- Phase 1: Client agent creates a job ---")
	job, err := market.Create(ctx, clientAgentID,
		"I need a high-quality 3D render of a futuristic city.",
		50.0, "USDC", evaluatorID, 0, 0,
		map[string]any{"style": "cyberpunk", "resolution": "4K"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "create failed:", err)
		os.Exit(1)
	}
	jobID, _ := job["job_id"].(string)
	if jobID == "" {
		jobID, _ = job["mission_id"].(string)
	}
	logger.Infof("Created job: %s", jobID)

	logger.Infof("--- Phase 2: Provider discovers and accepts ---")
	if ok, err := market.Accept(ctx, jobID, providerAgentID, 0); err != nil {
		fmt.Fprintln(os.Stderr, "accept failed:", err)
		os.Exit(1)
	} else if ok {
		logger.Infof("Provider %s accepted the job", providerAgentID)
	}

	logger.Infof("--- Phase 3: Provider submits deliverable ---")
	deliverable := map[string]any{
		"url":   "https://ipfs.io/ipfs/Qm...render.png",
		"proof": "rendering_logs_hash_xyz",
	}
	if ok, err := market.Submit(ctx, jobID, providerAgentID, deliverable, "Render complete.", 0); err != nil {
		fmt.Fprintln(os.Stderr, "submit failed:", err)
		os.Exit(1)
	} else if ok {
		logger.Infof("Provider submitted the deliverable")
	}

	logger.Infof("--- Phase 4: Evaluator completes and settles ---")
	if ok, err := market.Complete(ctx, jobID, evaluatorID, "The render meets all requirements.", 0); err != nil {
		fmt.Fprintln(os.Stderr, "complete failed:", err)
		os.Exit(1)
	} else if ok {
		logger.Infof("Evaluator approved. Payment released to provider!")
	}

	final, err := market.Get(ctx, jobID, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get failed:", err)
		os.Exit(1)
	}
	logger.Infof("Final job status: %v", final["status"])
}
