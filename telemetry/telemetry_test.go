package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewNoop(t *testing.T) {
	t.Parallel()

	tel := NewNoop()

	ctx, span := tel.StartInvokeAgent(context.Background(), InvokeAgentSpan{
		HarnessName: "agent",
		SessionID:   "s-1",
	})
	require.NotNil(t, ctx)
	require.False(t, span.IsRecording())

	_, chatSpan := tel.StartChat(ctx, ChatSpan{
		TurnIndex:      1,
		AttemptIndex:   1,
		RequestedModel: "gpt-4.1",
	})
	require.False(t, chatSpan.IsRecording())

	_, toolSpan := tel.StartExecuteTool(ctx, ExecuteToolSpan{
		TurnIndex:          1,
		ToolExecutionIndex: 1,
		ToolName:           "read",
	})
	require.False(t, toolSpan.IsRecording())

	span.End()
	chatSpan.End()
	toolSpan.End()
	tel.Shutdown(context.Background())
}

func TestNewStartsRootChatAndToolSpans(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tel := New(Config{TracerProvider: tp})

	rootCtx, rootSpan := tel.StartInvokeAgent(context.Background(), InvokeAgentSpan{
		HarnessName:     "agent",
		HarnessVersion:  "1.0.0",
		SessionID:       "s-1",
		ParentSessionID: "s-parent",
		ConversationID:  "c-1",
		AgentName:       "assistant",
		AgentVersion:    "2.0.0",
		AgentID:         "a-1",
	})

	chatCtx, chatSpan := tel.StartChat(rootCtx, ChatSpan{
		TurnIndex:      1,
		AttemptIndex:   2,
		ProviderName:   "openai",
		ProviderSystem: "openai",
		ProviderRoute:  "default",
		RequestedModel: "gpt-4.1",
		ResponseModel:  "gpt-4.1",
		ResolvedModel:  "gpt-4.1",
		ServerAddress:  "api.openai.com",
		ServerPort:     443,
	})

	toolCtx, toolSpan := tel.StartExecuteTool(chatCtx, ExecuteToolSpan{
		TurnIndex:          1,
		ToolExecutionIndex: 1,
		ToolName:           "read",
		ToolType:           "function",
		ToolCallID:         "call-1",
	})

	rootSpan.End()
	chatSpan.End()
	toolSpan.End()

	ended := recorder.Ended()
	require.Len(t, ended, 3)

	root := findSpan(t, ended, "invoke_agent agent")
	require.Equal(t, "invoke_agent", attrString(t, root.Attributes(), KeyOperationName))
	require.Equal(t, "agent", attrString(t, root.Attributes(), KeyHarnessName))
	require.Equal(t, "1.0.0", attrString(t, root.Attributes(), KeyHarnessVersion))
	require.Equal(t, "s-1", attrString(t, root.Attributes(), KeySessionID))
	require.Equal(t, "s-parent", attrString(t, root.Attributes(), KeyParentSessionID))
	require.Equal(t, "c-1", attrString(t, root.Attributes(), KeyConversationID))
	require.Equal(t, "assistant", attrString(t, root.Attributes(), KeyAgentName))
	require.Equal(t, "2.0.0", attrString(t, root.Attributes(), KeyAgentVersion))
	require.Equal(t, "a-1", attrString(t, root.Attributes(), KeyAgentID))

	chat := findSpan(t, ended, "chat gpt-4.1")
	require.Equal(t, root.SpanContext().TraceID(), chat.SpanContext().TraceID())
	require.Equal(t, root.SpanContext().SpanID(), chat.Parent().SpanID())
	require.Equal(t, "chat", attrString(t, chat.Attributes(), KeyOperationName))
	require.Equal(t, "agent", attrString(t, chat.Attributes(), KeyHarnessName))
	require.Equal(t, "s-1", attrString(t, chat.Attributes(), KeySessionID))
	require.Equal(t, "c-1", attrString(t, chat.Attributes(), KeyConversationID))
	require.Equal(t, int64(1), attrInt(t, chat.Attributes(), KeyTurnIndex))
	require.Equal(t, int64(2), attrInt(t, chat.Attributes(), KeyAttemptIndex))
	require.Equal(t, "openai", attrString(t, chat.Attributes(), KeyProviderName))
	require.Equal(t, "openai", attrString(t, chat.Attributes(), KeyProviderSystem))
	require.Equal(t, "default", attrString(t, chat.Attributes(), KeyProviderRoute))
	require.Equal(t, "gpt-4.1", attrString(t, chat.Attributes(), KeyRequestModel))
	require.Equal(t, "gpt-4.1", attrString(t, chat.Attributes(), KeyResponseModel))
	require.Equal(t, "gpt-4.1", attrString(t, chat.Attributes(), KeyProviderModelResolved))
	require.Equal(t, "api.openai.com", attrString(t, chat.Attributes(), KeyServerAddress))
	require.Equal(t, int64(443), attrInt(t, chat.Attributes(), KeyServerPort))

	tool := findSpan(t, ended, "execute_tool read")
	require.Equal(t, root.SpanContext().TraceID(), tool.SpanContext().TraceID())
	require.Equal(t, chat.SpanContext().SpanID(), tool.Parent().SpanID())
	require.Equal(t, "execute_tool", attrString(t, tool.Attributes(), KeyOperationName))
	require.Equal(t, "agent", attrString(t, tool.Attributes(), KeyHarnessName))
	require.Equal(t, "s-1", attrString(t, tool.Attributes(), KeySessionID))
	require.Equal(t, "c-1", attrString(t, tool.Attributes(), KeyConversationID))
	require.Equal(t, int64(1), attrInt(t, tool.Attributes(), KeyTurnIndex))
	require.Equal(t, int64(1), attrInt(t, tool.Attributes(), KeyToolExecutionIndex))
	require.Equal(t, "read", attrString(t, tool.Attributes(), KeyToolName))
	require.Equal(t, "function", attrString(t, tool.Attributes(), KeyToolType))
	require.Equal(t, "call-1", attrString(t, tool.Attributes(), KeyToolCallID))

	_ = toolCtx
}

func TestShutdownBestEffort(t *testing.T) {
	called := false
	logs, restore := captureTelemetryLogs(t)
	defer restore()

	tel := New(Config{
		Shutdown: func(context.Context) error {
			called = true
			return errors.New("boom")
		},
	})

	tel.Shutdown(context.Background())
	require.True(t, called)
	assert.Contains(t, logs.String(), "telemetry: shutdown failed")
	assert.Contains(t, logs.String(), "boom")
}

func TestRecordChatMetrics(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel := New(Config{MeterProvider: mp})
	recorder, ok := tel.(ChatMetricsRecorder)
	require.True(t, ok)

	recorder.RecordChatMetrics(context.Background(), ChatSpan{
		ProviderName:   "openai-compat",
		ProviderSystem: "openai",
		ProviderRoute:  "default",
		RequestedModel: "gpt-4o",
		ResponseModel:  "gpt-4o",
		ResolvedModel:  "gpt-4o",
		ServerAddress:  "api.openai.com",
		ServerPort:     443,
	}, ChatMetrics{
		ResponseModel: "gpt-4o",
		ResolvedModel: "gpt-4o",
		Usage: Usage{
			Input:  11,
			Output: 9,
		},
		Duration: 250 * time.Millisecond,
		Err:      errors.New("boom"),
	})

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	durationMetric := findMetric(t, rm, "gen_ai.client.operation.duration")
	durationPoints := histogramFloatPoints(t, durationMetric)
	require.Len(t, durationPoints, 1)
	assert.Equal(t, uint64(1), durationPoints[0].Count)
	assert.InDelta(t, 0.25, durationPoints[0].Sum, 1e-9)
	assert.Equal(t, "openai-compat", metricAttrString(t, durationPoints[0].Attributes, KeyProviderName))
	assert.Equal(t, "openai", metricAttrString(t, durationPoints[0].Attributes, KeyProviderSystem))
	assert.Equal(t, "gpt-4o", metricAttrString(t, durationPoints[0].Attributes, KeyRequestModel))
	assert.Equal(t, "gpt-4o", metricAttrString(t, durationPoints[0].Attributes, KeyResponseModel))
	assert.Equal(t, "gpt-4o", metricAttrString(t, durationPoints[0].Attributes, KeyProviderModelResolved))
	assert.True(t, strings.Contains(metricAttrString(t, durationPoints[0].Attributes, KeyErrorType), "errorString"))

	tokenMetric := findMetric(t, rm, "gen_ai.client.token.usage")
	tokenPoints := histogramIntPoints(t, tokenMetric)
	require.Len(t, tokenPoints, 2)

	inputPoint := pointByTokenType(t, tokenPoints, "input")
	assert.Equal(t, uint64(1), inputPoint.Count)
	assert.Equal(t, int64(11), inputPoint.Sum)
	assert.Equal(t, "openai-compat", metricAttrString(t, inputPoint.Attributes, KeyProviderName))
	assert.Equal(t, "input", metricAttrString(t, inputPoint.Attributes, KeyTokenType))
	assert.True(t, strings.Contains(metricAttrString(t, inputPoint.Attributes, KeyErrorType), "errorString"))

	outputPoint := pointByTokenType(t, tokenPoints, "output")
	assert.Equal(t, uint64(1), outputPoint.Count)
	assert.Equal(t, int64(9), outputPoint.Sum)
	assert.Equal(t, "output", metricAttrString(t, outputPoint.Attributes, KeyTokenType))
}

func findSpan(t *testing.T, ended []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range ended {
		if span.Name() == name {
			return span
		}
	}

	require.Failf(t, "span not found", "missing span %q", name)
	var zero sdktrace.ReadOnlySpan
	return zero
}

func attrString(t *testing.T, attrs []attribute.KeyValue, key string) string {
	t.Helper()

	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}

	require.Failf(t, "attribute not found", "missing %q", key)
	return ""
}

func attrInt(t *testing.T, attrs []attribute.KeyValue, key string) int64 {
	t.Helper()

	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsInt64()
		}
	}

	require.Failf(t, "attribute not found", "missing %q", key)
	return 0
}

func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return metric
			}
		}
	}

	require.Failf(t, "metric not found", "missing %q", name)
	return metricdata.Metrics{}
}

func histogramFloatPoints(t *testing.T, metric metricdata.Metrics) []metricdata.HistogramDataPoint[float64] {
	t.Helper()

	hist, ok := metric.Data.(metricdata.Histogram[float64])
	require.Truef(t, ok, "unexpected metric type %T", metric.Data)
	return hist.DataPoints
}

func histogramIntPoints(t *testing.T, metric metricdata.Metrics) []metricdata.HistogramDataPoint[int64] {
	t.Helper()

	hist, ok := metric.Data.(metricdata.Histogram[int64])
	require.Truef(t, ok, "unexpected metric type %T", metric.Data)
	return hist.DataPoints
}

func pointByTokenType(t *testing.T, points []metricdata.HistogramDataPoint[int64], tokenType string) metricdata.HistogramDataPoint[int64] {
	t.Helper()

	for _, point := range points {
		if metricAttrString(t, point.Attributes, KeyTokenType) == tokenType {
			return point
		}
	}

	require.Failf(t, "metric point not found", "missing token type %q", tokenType)
	return metricdata.HistogramDataPoint[int64]{}
}

func captureTelemetryLogs(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()

	var buf bytes.Buffer
	prev := slog.Default()
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
	return &buf, func() {
		slog.SetDefault(prev)
	}
}

func metricAttrString(t *testing.T, attrs attribute.Set, key string) string {
	t.Helper()

	value, ok := attrs.Value(attribute.Key(key))
	if !ok {
		require.Failf(t, "metric attribute not found", "missing %q", key)
		return ""
	}
	return value.AsString()
}
