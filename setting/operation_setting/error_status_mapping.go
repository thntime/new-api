package operation_setting

import (
	"fmt"
	"regexp"

	"github.com/QuantumNous/new-api/common"
)

type ErrorStatusMappingMatch struct {
	StatusCode   int    `json:"status_code"`
	MessageRegex string `json:"message_regex"`
}

type ErrorStatusMappingHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ErrorStatusMappingOutput struct {
	StatusCode int                        `json:"status_code"`
	Message    string                     `json:"message"`
	Type       string                     `json:"type"`
	Code       string                     `json:"code"`
	Headers    []ErrorStatusMappingHeader `json:"headers"`
}

type ErrorStatusMappingRule struct {
	Name    string                   `json:"name"`
	Enabled bool                     `json:"enabled"`
	Match   ErrorStatusMappingMatch  `json:"match"`
	Output  ErrorStatusMappingOutput `json:"output"`
	regex   *regexp.Regexp           `json:"-"`
}

const defaultErrorStatusMappingRulesJSON = `[{"name":"Rate limit","enabled":false,"match":{"status_code":0,"message_regex":"exhausted|tpm/rpm|rps|rate limit"},"output":{"status_code":429,"message":"rate limit exceeded","type":"rate_limit_error","code":"rate_limit_exceeded","headers":[{"key":"x-should-retry","value":"true"},{"key":"Retry-After","value":"30"}]}}]`

var ErrorStatusMappingRules []ErrorStatusMappingRule

func init() {
	ErrorStatusMappingRules, _ = ParseErrorStatusMappingRules(defaultErrorStatusMappingRulesJSON)
}

func ParseErrorStatusMappingRules(s string) ([]ErrorStatusMappingRule, error) {
	var rules []ErrorStatusMappingRule
	if err := common.UnmarshalJsonStr(s, &rules); err != nil {
		return nil, fmt.Errorf("invalid error status mapping rules: %w", err)
	}
	for i := range rules {
		rule := &rules[i]
		if rule.Match.StatusCode != 0 && (rule.Match.StatusCode < 100 || rule.Match.StatusCode > 599) {
			return nil, fmt.Errorf("rule %q: match status_code must be 0 or between 100 and 599", rule.Name)
		}
		if rule.Output.StatusCode < 100 || rule.Output.StatusCode > 599 {
			return nil, fmt.Errorf("rule %q: output status_code must be between 100 and 599", rule.Name)
		}
		if rule.Match.MessageRegex != "" {
			compiled, err := regexp.Compile("(?i)" + rule.Match.MessageRegex)
			if err != nil {
				return nil, fmt.Errorf("rule %q: invalid message regex: %w", rule.Name, err)
			}
			rule.regex = compiled
		}
	}
	return rules, nil
}

func ErrorStatusMappingRulesFromString(s string) error {
	if s == "" {
		ErrorStatusMappingRules = nil
		return nil
	}
	rules, err := ParseErrorStatusMappingRules(s)
	if err != nil {
		return err
	}
	ErrorStatusMappingRules = rules
	return nil
}

func ErrorStatusMappingRulesToString() string {
	data, err := common.Marshal(ErrorStatusMappingRules)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func MatchErrorStatusMapping(statusCode int, message string) *ErrorStatusMappingOutput {
	for i := range ErrorStatusMappingRules {
		rule := &ErrorStatusMappingRules[i]
		if !rule.Enabled {
			continue
		}
		if rule.Match.StatusCode != 0 && rule.Match.StatusCode != statusCode {
			continue
		}
		if rule.regex != nil && !rule.regex.MatchString(message) {
			continue
		}
		return &rule.Output
	}
	return nil
}
