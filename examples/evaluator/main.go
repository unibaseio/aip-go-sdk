// Command evaluator is the "brain" of an AI evaluator role, mirroring the
// Python examples/evaluator_logic.py: fetch a submitted job, run verification
// logic, then settle on-chain via complete (or reject).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/unibaseio/unibase-aip-sdk-go/commerce"
	"github.com/unibaseio/unibase-aip-sdk-go/platform"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// autoVerifyAgentWork monitors a submitted job, verifies the deliverable, and
// calls complete/reject on the ERC-8183 market.
func autoVerifyAgentWork(ctx context.Context, market *commerce.JobClient, jobID, evaluatorID string) error {
	job, err := market.Get(ctx, jobID, 0)
	if err != nil {
		return err
	}
	if status, _ := job["status"].(string); status != "submitted" {
		fmt.Println("Job not in submitted state.")
		return nil
	}

	deliverableURI, _ := job["deliverable_uri"].(string)
	fmt.Printf("Verifying work at: %s\n", deliverableURI)

	// --- AUTOMATED VERIFICATION LOGIC ---
	// If it's code: run unit tests. If it's an image: run aesthetics scoring.
	// If it's a swap: check the chain for the transaction hash. Optionally verify
	// the downloaded file's sha256 matches job["deliverable_hash"].
	isValid := true // placeholder for real logic

	if isValid {
		fmt.Println("Verification passed! Releasing payment...")
		_, err := market.Complete(ctx, jobID, evaluatorID,
			"Verified: Deliverable matches requirements and hash is valid.", 0)
		return err
	}
	fmt.Println("Verification failed! Rejecting...")
	_, err = market.Client().RejectJob(ctx, jobID, evaluatorID,
		"Failed: Quality below threshold or invalid format.")
	return err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: evaluator <job_id>")
		fmt.Println("(this demo represents the evaluator's verification + settlement logic)")
		os.Exit(1)
	}
	jobID := os.Args[1]
	evaluatorID := getenv("EVALUATOR_ID", "0x70997970C51812dc3A010C7d01b50e0d17dc79C8")

	client := platform.New(getenv("AIP_PLATFORM_URL", "http://localhost:8000"))
	market := commerce.NewJobClient(client)

	if err := autoVerifyAgentWork(context.Background(), market, jobID, evaluatorID); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
