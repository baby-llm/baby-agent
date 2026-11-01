package observe

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("babyagent/ch11")

func StartAgentSpan(ctx context.Context, conversationID, messageID string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "agent.query", trace.WithAttributes(attribute.String("conversation.id", conversationID), attribute.String("message.id", messageID)))
}
func StartLLMSpan(ctx context.Context, model string, loop int) (context.Context, trace.Span) {
	return tracer.Start(ctx, "llm.call", trace.WithAttributes(attribute.String("gen_ai.request.model", model), attribute.Int("agent.loop", loop)))
}
func StartToolSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "tool.call", trace.WithAttributes(attribute.String("tool.name", name)))
}
