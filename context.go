package aegis

import (
	"context"
	"time"
)

type Subject struct {
	ID           string
	Type         string
	Roles        []string
	Capabilities []CapabilityRef
	Attributes   map[string]any
}

type ExecutionContext struct {
	Operation            string
	Module               string
	Subject              Subject
	Capabilities         GrantedCapabilities
	CapabilityResolution CapabilityResolution
	Resources            ResourceResolver
	Deadline             time.Time
	Metadata             map[string]any
	RequestID            string
	TraceID              string
	Environment          string
	Transport            string
	TenantID             string
	DegradationMode      DegradationMode
	Decision             Decision
}

type contextKey string

const (
	subjectContextKey      contextKey = "aegis:subject"
	capabilitiesContextKey contextKey = "aegis:capabilities"
	metadataContextKey     contextKey = "aegis:metadata"
	requestIDContextKey    contextKey = "aegis:request-id"
	traceIDContextKey      contextKey = "aegis:trace-id"
	environmentContextKey  contextKey = "aegis:environment"
	transportContextKey    contextKey = "aegis:transport"
	tenantIDContextKey     contextKey = "aegis:tenant-id"
	degradationContextKey  contextKey = "aegis:degradation"
	confirmedContextKey    contextKey = "aegis:confirmed"
)

type grantedCapabilitiesContextValue struct {
	caps     GrantedCapabilities
	explicit bool
}

func WithSubject(ctx context.Context, subject Subject) context.Context {
	subject.Roles = cloneStringSlice(subject.Roles)
	subject.Capabilities = cloneCapabilitySlice(subject.Capabilities)
	subject.Attributes = cloneMap(subject.Attributes)
	return context.WithValue(ctx, subjectContextKey, subject)
}

func SubjectFromContext(ctx context.Context) Subject {
	subject, _ := ctx.Value(subjectContextKey).(Subject)
	subject.Roles = cloneStringSlice(subject.Roles)
	subject.Capabilities = cloneCapabilitySlice(subject.Capabilities)
	subject.Attributes = cloneMap(subject.Attributes)
	return subject
}

func WithCapabilities(ctx context.Context, caps GrantedCapabilities) context.Context {
	return context.WithValue(ctx, capabilitiesContextKey, grantedCapabilitiesContextValue{
		caps:     caps.clone(),
		explicit: true,
	})
}

func WithCapabilityRefs(ctx context.Context, refs ...CapabilityRef) context.Context {
	return WithCapabilities(ctx, NewGrantedCapabilities(refs...))
}

func GrantedCapabilitiesFromContext(ctx context.Context) GrantedCapabilities {
	caps, _ := grantedCapabilitiesStateFromContext(ctx)
	return caps.clone()
}

func grantedCapabilitiesStateFromContext(ctx context.Context) (GrantedCapabilities, bool) {
	switch value := ctx.Value(capabilitiesContextKey).(type) {
	case grantedCapabilitiesContextValue:
		return value.caps.clone(), value.explicit
	case GrantedCapabilities:
		return value.clone(), true
	default:
		return GrantedCapabilities{values: map[CapabilityRef]struct{}{}}, false
	}
}

func WithMetadata(ctx context.Context, metadata map[string]any) context.Context {
	return context.WithValue(ctx, metadataContextKey, cloneMap(metadata))
}

func MetadataFromContext(ctx context.Context) map[string]any {
	metadata, _ := ctx.Value(metadataContextKey).(map[string]any)
	return cloneMap(metadata)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(traceIDContextKey).(string)
	return value
}

func WithEnvironment(ctx context.Context, environment string) context.Context {
	return context.WithValue(ctx, environmentContextKey, environment)
}

func EnvironmentFromContext(ctx context.Context) string {
	value, _ := ctx.Value(environmentContextKey).(string)
	return value
}

func WithTransport(ctx context.Context, transport string) context.Context {
	return context.WithValue(ctx, transportContextKey, transport)
}

func TransportFromContext(ctx context.Context) string {
	value, _ := ctx.Value(transportContextKey).(string)
	return value
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDContextKey, tenantID)
}

func TenantIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(tenantIDContextKey).(string)
	return value
}

func WithDegradationMode(ctx context.Context, mode DegradationMode) context.Context {
	return context.WithValue(ctx, degradationContextKey, mode)
}

func DegradationModeFromContext(ctx context.Context) DegradationMode {
	value, _ := ctx.Value(degradationContextKey).(DegradationMode)
	return value
}

func WithConfirmed(ctx context.Context, confirmed bool) context.Context {
	return context.WithValue(ctx, confirmedContextKey, confirmed)
}

func ConfirmedFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(confirmedContextKey).(bool)
	return value
}

func deadlineFromContext(ctx context.Context) time.Time {
	deadline, ok := ctx.Deadline()
	if !ok {
		return time.Time{}
	}
	return deadline
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}

	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneCapabilitySlice(in []CapabilityRef) []CapabilityRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]CapabilityRef, len(in))
	copy(out, in)
	return out
}
