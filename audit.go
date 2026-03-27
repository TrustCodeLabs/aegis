package aegis

import (
	"context"
	"time"
)

type AuditEvent struct {
	Operation  string
	Module     string
	Subject    Subject
	Decision   Decision
	Allowed    bool
	Success    bool
	Effects    []EffectRecord
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
}

type AuditWriter interface {
	Write(ctx context.Context, event AuditEvent) error
}

type NoopAuditWriter struct{}

func (NoopAuditWriter) Write(ctx context.Context, event AuditEvent) error {
	return nil
}

type ExecutionReport struct {
	Execution         ExecutionContext
	Decision          Decision
	PolicyExplanation DecisionExplanation
	Effects           []EffectRecord
	StartedAt         time.Time
	FinishedAt        time.Time
	Err               error
	AuditErr          error
}

func (r ExecutionReport) AuditEvent() AuditEvent {
	var errMessage string
	if r.Err != nil {
		errMessage = r.Err.Error()
	}
	if r.AuditErr != nil && errMessage == "" {
		errMessage = r.AuditErr.Error()
	}

	return AuditEvent{
		Operation:  r.Execution.Operation,
		Module:     r.Execution.Module,
		Subject:    r.Execution.Subject,
		Decision:   r.Decision,
		Allowed:    !IsCode(r.Err, CodeCapabilityDenied) && !IsCode(r.Err, CodePolicyDenied) && !IsCode(r.Err, CodeConfirmationNeeded),
		Success:    r.Err == nil && r.AuditErr == nil,
		Effects:    r.Effects,
		Error:      errMessage,
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
	}
}
