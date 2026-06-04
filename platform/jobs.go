package platform

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// CreateJob creates a new agentic job. expiresIn defaults to 86400 when 0.
func (c *Client) CreateJob(ctx context.Context, clientID, description string, rewardAmount float64, rewardToken, evaluatorID string, expiresIn int, metadata map[string]any) (map[string]any, error) {
	if expiresIn == 0 {
		expiresIn = 86400
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload := map[string]any{
		"description":   description,
		"reward_amount": rewardAmount,
		"reward_token":  rewardToken,
		"evaluator_id":  evaluatorID,
		"expires_in":    expiresIn,
		"metadata":      metadata,
	}
	q := url.Values{"client_id": {clientID}}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/jobs", q, payload, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AcceptJob accepts a job as a provider.
func (c *Client) AcceptJob(ctx context.Context, jobID, providerID string, chainID int) (map[string]any, error) {
	q := url.Values{"provider_id": {providerID}}
	if chainID != 0 {
		q.Set("chain_id", strconv.Itoa(chainID))
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/accept", q, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SubmitJobWork submits work (a deliverable) for a job.
func (c *Client) SubmitJobWork(ctx context.Context, jobID, providerID string, deliverableData any, description string, chainID int) (map[string]any, error) {
	payload := map[string]any{
		"provider_id":      providerID,
		"deliverable_data": deliverableData,
		"description":      description,
	}
	q := url.Values{}
	if chainID != 0 {
		q.Set("chain_id", strconv.Itoa(chainID))
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/submit", q, payload, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CompleteJob completes a job as the evaluator.
func (c *Client) CompleteJob(ctx context.Context, jobID, evaluatorID, reason string, chainID int) (map[string]any, error) {
	payload := map[string]any{"evaluator_id": evaluatorID, "reason": reason}
	q := url.Values{}
	if chainID != 0 {
		q.Set("chain_id", strconv.Itoa(chainID))
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/complete", q, payload, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RejectJob rejects a submitted job as the given rejector.
func (c *Client) RejectJob(ctx context.Context, jobID, rejectorID, reason string) (map[string]any, error) {
	q := url.Values{"rejector_id": {rejectorID}}
	var out map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/reject", q, map[string]any{"reason": reason}, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetJob returns details for a job.
func (c *Client) GetJob(ctx context.Context, jobID string, chainID int) (map[string]any, error) {
	q := url.Values{}
	if chainID != 0 {
		q.Set("chain_id", strconv.Itoa(chainID))
	}
	var out map[string]any
	if err := c.do(ctx, http.MethodGet, "/v1/jobs/"+jobID, q, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
