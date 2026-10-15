package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseErrorStatusMappingRules(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		rules, err := ParseErrorStatusMappingRules(`[{"name":"r1","enabled":true,"match":{"status_code":429,"message_regex":"boom"},"output":{"status_code":503,"message":"oops","type":"t","code":"c","headers":[{"key":"k","value":"v"}]}}]`)
		require.NoError(t, err)
		require.Len(t, rules, 1)
		assert.Equal(t, "r1", rules[0].Name)
		assert.True(t, rules[0].Enabled)
		assert.Equal(t, 429, rules[0].Match.StatusCode)
		assert.Equal(t, "boom", rules[0].Match.MessageRegex)
		assert.Equal(t, 503, rules[0].Output.StatusCode)
		assert.Equal(t, "oops", rules[0].Output.Message)
		assert.Equal(t, "t", rules[0].Output.Type)
		assert.Equal(t, "c", rules[0].Output.Code)
		require.Len(t, rules[0].Output.Headers, 1)
		assert.Equal(t, "k", rules[0].Output.Headers[0].Key)
		assert.Equal(t, "v", rules[0].Output.Headers[0].Value)
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseErrorStatusMappingRules(`[`)
		require.Error(t, err)
	})

	t.Run("invalid regex", func(t *testing.T) {
		_, err := ParseErrorStatusMappingRules(`[{"name":"bad","match":{"message_regex":"["}}]`)
		require.Error(t, err)
	})

	t.Run("match status code out of range", func(t *testing.T) {
		for _, raw := range []string{`99`, `600`} {
			_, err := ParseErrorStatusMappingRules(`[{"name":"bad","match":{"status_code":` + raw + `}}]`)
			require.Error(t, err, raw)
		}
	})

	t.Run("output status code out of range", func(t *testing.T) {
		for _, raw := range []string{`99`, `600`} {
			_, err := ParseErrorStatusMappingRules(`[{"name":"bad","output":{"status_code":` + raw + `}}]`)
			require.Error(t, err, raw)
		}
	})

	t.Run("match status code zero means any", func(t *testing.T) {
		rules, err := ParseErrorStatusMappingRules(`[{"name":"any","match":{"status_code":0,"message_regex":"boom"},"output":{"status_code":429}}]`)
		require.NoError(t, err)
		require.Len(t, rules, 1)
		assert.Equal(t, 0, rules[0].Match.StatusCode)
	})
}

func TestErrorStatusMappingRulesFromString(t *testing.T) {
	original := ErrorStatusMappingRules
	t.Cleanup(func() { ErrorStatusMappingRules = original })

	ErrorStatusMappingRules = []ErrorStatusMappingRule{{Name: "existing"}}
	require.NoError(t, ErrorStatusMappingRulesFromString(""))
	assert.Nil(t, ErrorStatusMappingRules)

	require.Error(t, ErrorStatusMappingRulesFromString(`[{"name":"bad","match":{"message_regex":"["}}]`))
	assert.Nil(t, ErrorStatusMappingRules, "invalid input must not override the current value")
}

func TestDefaultErrorStatusMappingRules(t *testing.T) {
	require.Len(t, ErrorStatusMappingRules, 1)
	rule := ErrorStatusMappingRules[0]
	assert.False(t, rule.Enabled)
	assert.Equal(t, "exhausted|tpm/rpm|rps|rate limit", rule.Match.MessageRegex)
	assert.Equal(t, 429, rule.Output.StatusCode)

	headers := map[string]string{}
	for _, header := range rule.Output.Headers {
		headers[header.Key] = header.Value
	}
	assert.Equal(t, "true", headers["x-should-retry"])
	assert.Equal(t, "30", headers["Retry-After"])
}

func TestMatchErrorStatusMapping(t *testing.T) {
	original := ErrorStatusMappingRules
	t.Cleanup(func() { ErrorStatusMappingRules = original })

	setRules := func(t *testing.T, raw string) {
		t.Helper()
		rules, err := ParseErrorStatusMappingRules(raw)
		require.NoError(t, err)
		ErrorStatusMappingRules = rules
	}

	require.NoError(t, ErrorStatusMappingRulesFromString(defaultErrorStatusMappingRulesJSON))
	ErrorStatusMappingRules[0].Enabled = true

	t.Run("hit on upstream 200 with error body", func(t *testing.T) {
		output := MatchErrorStatusMapping(200, "inference exceeds tpm/rpm limit")
		require.NotNil(t, output)
		assert.Equal(t, 429, output.StatusCode)
		assert.Equal(t, "rate_limit_exceeded", output.Code)
	})

	t.Run("case insensitive", func(t *testing.T) {
		require.NotNil(t, MatchErrorStatusMapping(200, "TPM EXHAUSTED"))
	})

	t.Run("status code zero matches any", func(t *testing.T) {
		setRules(t, `[{"name":"any","enabled":true,"match":{"status_code":0,"message_regex":"boom"},"output":{"status_code":429}}]`)
		require.NotNil(t, MatchErrorStatusMapping(418, "boom"))
		require.NotNil(t, MatchErrorStatusMapping(200, "boom"))
	})

	t.Run("status code mismatch does not match", func(t *testing.T) {
		setRules(t, `[{"name":"s500","enabled":true,"match":{"status_code":500,"message_regex":"exhausted"},"output":{"status_code":429}}]`)
		assert.Nil(t, MatchErrorStatusMapping(429, "rpm exhausted"))
	})

	t.Run("message mismatch does not match", func(t *testing.T) {
		setRules(t, `[{"name":"boom","enabled":true,"match":{"status_code":500,"message_regex":"boom"},"output":{"status_code":429}}]`)
		assert.Nil(t, MatchErrorStatusMapping(500, "all good"))
	})

	t.Run("first matching rule wins", func(t *testing.T) {
		setRules(t, `[{"name":"first","enabled":true,"match":{"message_regex":"exhausted"},"output":{"status_code":429,"code":"first"}},{"name":"second","enabled":true,"match":{"message_regex":"exhausted"},"output":{"status_code":503,"code":"second"}}]`)
		output := MatchErrorStatusMapping(500, "tpm exhausted")
		require.NotNil(t, output)
		assert.Equal(t, "first", output.Code)
	})

	t.Run("disabled rules are skipped", func(t *testing.T) {
		setRules(t, `[{"name":"off","enabled":false,"match":{"message_regex":"exhausted"},"output":{"status_code":429}}]`)
		assert.Nil(t, MatchErrorStatusMapping(500, "tpm exhausted"))
	})

	t.Run("default output avoids quota wording", func(t *testing.T) {
		ErrorStatusMappingRules = original
		rule := ErrorStatusMappingRules[0]
		assert.Equal(t, "rate_limit_exceeded", rule.Output.Code)
		for _, word := range []string{"insufficient_quota", "insufficient balance", "quota exceeded", "budget exceeded", "usage limit", "limit exhausted", "package has expired", "plan does not yet include"} {
			assert.NotContains(t, rule.Output.Message, word)
		}
	})
}
