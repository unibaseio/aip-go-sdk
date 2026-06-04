// Command auto_submit_agent shows an agent that automatically submits its
// results to the commerce layer, mirroring the Python
// examples/automatic_submit_agent.py. The Python version injects
// submit_commerce_work via the callback-based AgentContext; the Go SDK uses an
// explicit commerce.JobClient instead.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/unibaseio/unibase-aip-sdk-go/commerce"
	"github.com/unibaseio/unibase-aip-sdk-go/internal/log"
	"github.com/unibaseio/unibase-aip-sdk-go/platform"
	"github.com/unibaseio/unibase-aip-sdk-go/types"
)

var logger = log.Get("examples.auto_submit_agent")

// SearchAndSummaryAgent performs a search/summary task and, when the task was
// triggered by a commerce job, submits the deliverable to the job market.
type SearchAndSummaryAgent struct {
	AgentID string
	market  *commerce.JobClient
}

func (a *SearchAndSummaryAgent) PerformTask(ctx context.Context, task types.Task) (*types.TaskResult, error) {
	logger.Infof("Received task: %s", task.Name)

	// 1. Execute the actual work (AI logic would go here).
	resultData := map[string]any{
		"summary": "Latest news on BNB Chain: Market cap reached new highs...",
		"sources": []string{"https://news.bnb", "https://twitter.com/bnbchain"},
	}

	// 2. If this task came from a commerce job, submit the deliverable.
	if jobID, _ := task.Payload["job_id"].(string); jobID != "" && a.market != nil {
		logger.Infof("Job ID %s detected. Submitting results to commerce layer...", jobID)
		ok, err := a.market.Submit(ctx, jobID, a.AgentID, resultData, "Search and summary task complete.", 0)
		if err != nil {
			logger.Warnf("Failed to submit commerce results for Job %s: %v", jobID, err)
		} else if ok {
			logger.Infof("Successfully submitted results for Job %s to the blockchain.", jobID)
		}
	}

	// 3. Return the standard task result.
	return types.SuccessResult(resultData, "Processed news search and submitted to commerce layer.", nil), nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	client := platform.New(getenv("AIP_PLATFORM_URL", "http://localhost:8000"))
	agent := &SearchAndSummaryAgent{
		AgentID: "agent_uuid_searcher_123",
		market:  commerce.NewJobClient(client),
	}
	logger.Infof("Agent is ready with automatic commerce submission support.")

	// Demonstrate a task that arrived via a commerce job.
	task := types.Task{
		TaskID:  "task-1",
		Name:    "search_and_summary",
		Payload: map[string]any{"job_id": getenv("DEMO_JOB_ID", "")},
	}
	result, err := agent.PerformTask(context.Background(), task)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("Result: success=%v summary=%q\n", result.Success, result.Summary)
}
