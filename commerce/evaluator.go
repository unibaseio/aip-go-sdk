package commerce

import (
	"context"

	"github.com/unibaseio/unibase-aip-sdk-go/internal/log"
)

var evalLogger = log.Get("commerce.evaluator")

// SchemaEvaluator is a Tier-2 evaluator that verifies job deliverables against
// a JSON-schema-like specification and triggers on-chain completion/rejection.
type SchemaEvaluator struct {
	market      *JobClient
	evaluatorID string
}

// NewSchemaEvaluator creates a SchemaEvaluator for the given job market.
func NewSchemaEvaluator(market *JobClient, evaluatorID string) *SchemaEvaluator {
	return &SchemaEvaluator{market: market, evaluatorID: evaluatorID}
}

// VerifyAndSettle fetches the job, validates its deliverable data against the
// deliverable schema, and completes or rejects the job accordingly.
func (e *SchemaEvaluator) VerifyAndSettle(ctx context.Context, jobID string, requirementSchema, deliverableSchema map[string]any) (bool, error) {
	job, err := e.market.Get(ctx, jobID, 0)
	if err != nil {
		evalLogger.Errorf("Error during automated evaluation: %v", err)
		return false, nil
	}
	if status, _ := job["status"].(string); status != "submitted" {
		evalLogger.Warnf("Job %s is not in 'submitted' state. Current: %v", jobID, job["status"])
		return false, nil
	}

	var deliverableData any
	if md, ok := job["metadata"].(map[string]any); ok {
		deliverableData = md["deliverable_data"]
	}
	if deliverableData == nil {
		_, _ = e.reject(ctx, jobID, "Missing deliverable data in job metadata.")
		return false, nil
	}

	if validateData(deliverableData, deliverableSchema) {
		evalLogger.Infof("Job %s passed verification.", jobID)
		return e.market.Complete(ctx, jobID, e.evaluatorID, "Automated verification successful: Deliverable matches required schema.", 0)
	}
	evalLogger.Warnf("Job %s failed verification.", jobID)
	ok, _ := e.reject(ctx, jobID, "Deliverable does not match the required schema.")
	return ok, nil
}

// validateData performs basic type and required-key validation.
func validateData(data any, schema map[string]any) bool {
	if t, _ := schema["type"].(string); t == "object" {
		m, ok := data.(map[string]any)
		if !ok {
			return false
		}
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				k, _ := key.(string)
				if _, present := m[k]; !present {
					return false
				}
			}
		}
	}
	return true
}

func (e *SchemaEvaluator) reject(ctx context.Context, jobID, reason string) (bool, error) {
	if _, err := e.market.Client().RejectJob(ctx, jobID, e.evaluatorID, reason); err != nil {
		return false, err
	}
	return true, nil
}
