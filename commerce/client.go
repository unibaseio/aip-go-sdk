// Package commerce provides a specialized client for Agentic Commerce (Jobs)
// and an automated schema evaluator, mirroring aip_sdk/commerce.
package commerce

import (
	"context"

	"github.com/unibaseio/aip-go-sdk/platform"
)

// JobClient is a specialized client for Agentic Commerce, wrapping a platform Client.
type JobClient struct {
	aip *platform.Client
}

// NewJobClient creates a JobClient backed by the given platform client.
func NewJobClient(aip *platform.Client) *JobClient { return &JobClient{aip: aip} }

// Client returns the underlying platform client.
func (j *JobClient) Client() *platform.Client { return j.aip }

// Create creates a new job. expiresIn defaults to 86400 when 0; a non-zero
// chainID is folded into the metadata.
func (j *JobClient) Create(ctx context.Context, clientID, description string, rewardAmount float64, rewardToken, evaluatorID string, expiresIn, chainID int, metadata map[string]any) (map[string]any, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if chainID != 0 {
		metadata["chain_id"] = chainID
	}
	return j.aip.CreateJob(ctx, clientID, description, rewardAmount, rewardToken, evaluatorID, expiresIn, metadata)
}

// Accept accepts a job, returning true when the gateway reports "accepted".
func (j *JobClient) Accept(ctx context.Context, jobID, providerID string, chainID int) (bool, error) {
	result, err := j.aip.AcceptJob(ctx, jobID, providerID, chainID)
	if err != nil {
		return false, err
	}
	return statusEquals(result, "accepted"), nil
}

// Submit submits work for a job, returning true when reported "submitted".
func (j *JobClient) Submit(ctx context.Context, jobID, providerID string, deliverableData any, description string, chainID int) (bool, error) {
	result, err := j.aip.SubmitJobWork(ctx, jobID, providerID, deliverableData, description, chainID)
	if err != nil {
		return false, err
	}
	return statusEquals(result, "submitted"), nil
}

// Complete completes a job as evaluator, returning true when reported "completed".
func (j *JobClient) Complete(ctx context.Context, jobID, evaluatorID, reason string, chainID int) (bool, error) {
	result, err := j.aip.CompleteJob(ctx, jobID, evaluatorID, reason, chainID)
	if err != nil {
		return false, err
	}
	return statusEquals(result, "completed"), nil
}

// Get returns a job's details and current status.
func (j *JobClient) Get(ctx context.Context, jobID string, chainID int) (map[string]any, error) {
	return j.aip.GetJob(ctx, jobID, chainID)
}

func statusEquals(result map[string]any, want string) bool {
	s, _ := result["status"].(string)
	return s == want
}
