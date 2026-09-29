package telemetry

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectTraceparent_NoSpan(t *testing.T) {
	env := []string{"PATH=/usr/bin"}
	result := InjectTraceparent(context.Background(), env)
	assert.Equal(t, env, result)
}

func TestInjectTraceparent_ValidSpan(t *testing.T) {
	tp, err := InitTracer("prop-service", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := Tracer().Start(context.Background(), "prop-span")
	defer span.End()

	env := []string{"PATH=/usr/bin", "FOO=BAR"}
	result := InjectTraceparent(ctx, env)

	var traceparentFound bool
	for _, e := range result {
		if strings.HasPrefix(e, "TRACEPARENT=") {
			traceparentFound = true
			parts := strings.Split(strings.TrimPrefix(e, "TRACEPARENT="), "-")
			assert.Len(t, parts, 4, "W3C traceparent must have 4 dash-separated parts")
			assert.Equal(t, "00", parts[0], "version must be 00")
			assert.Len(t, parts[1], 32, "trace id must be 32 hex chars")
			assert.Len(t, parts[2], 16, "parent span id must be 16 hex chars")
		}
	}
	assert.True(t, traceparentFound, "TRACEPARENT env var should be injected into environment")
}

func TestSensitiveAttrRedaction(t *testing.T) {
	attr1 := Attr("api_key", "secret-token-12345")
	assert.Equal(t, "[REDACTED]", attr1.Value.AsString())

	attr2 := Attr("Authorization", "Bearer sensitive")
	assert.Equal(t, "[REDACTED]", attr2.Value.AsString())

	attr3 := Attr("normal_key", "public_value")
	assert.Equal(t, "public_value", attr3.Value.AsString())

	attr4 := AttrInt("secret_code", 42)
	assert.Equal(t, "[REDACTED]", attr4.Value.AsString())

	attr5 := AttrInt("item_count", 42)
	assert.Equal(t, int64(42), attr5.Value.AsInt64())
}
