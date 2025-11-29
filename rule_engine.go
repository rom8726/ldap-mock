package main

import (
	"regexp"
	"sort"
	"strings"
)

type RuleEngine struct {
	rules []Rule
}

func NewRuleEngine(rules []Rule) *RuleEngine {
	sortedRules := make([]Rule, len(rules))
	copy(sortedRules, rules)

	sort.Slice(sortedRules, func(i, j int) bool {
		return sortedRules[i].Priority > sortedRules[j].Priority
	})

	return &RuleEngine{rules: sortedRules}
}

type LDAPScope int

const (
	ScopeBase LDAPScope = iota
	ScopeOne
	ScopeSub
)

func ParseScope(s string) LDAPScope {
	switch strings.ToLower(s) {
	case "base":
		return ScopeBase
	case "one":
		return ScopeOne
	case "sub":
		return ScopeSub
	default:
		return ScopeSub
	}
}

func (s LDAPScope) String() string {
	switch s {
	case ScopeBase:
		return "base"
	case ScopeOne:
		return "one"
	case ScopeSub:
		return "sub"
	default:
		return "sub"
	}
}

type SearchRequest struct {
	BaseDN string
	Scope  LDAPScope
	Filter string
}

func (e *RuleEngine) FindMatchingRule(req SearchRequest) *Rule {
	for i := range e.rules {
		rule := &e.rules[i]

		if rule.BaseDN != "" && !strings.EqualFold(rule.BaseDN, req.BaseDN) {
			continue
		}

		if rule.Scope != "" && ParseScope(rule.Scope) != req.Scope {
			continue
		}

		if !matchRuleFilter(rule.Filter, req.Filter) {
			continue
		}

		return rule
	}

	return nil
}

func matchRuleFilter(ruleFilter, reqFilter string) bool {
	ruleF, err := ParseFilter(ruleFilter)
	if err != nil {
		return false
	}

	reqF, err := ParseFilter(reqFilter)
	if err != nil {
		return false
	}

	return filtersMatch(ruleF, reqF)
}

func filtersMatch(rule, req *Filter) bool {
	if rule.Type != req.Type {
		if rule.Type == FilterOr {
			for _, child := range rule.Children {
				if filtersMatch(child, req) {
					return true
				}
			}
			return false
		}

		if req.Type == FilterAnd {
			for _, child := range req.Children {
				if filtersMatch(rule, child) {
					return true
				}
			}
			return false
		}

		return false
	}

	switch rule.Type {
	case FilterAnd, FilterOr:
		if len(rule.Children) > len(req.Children) {
			return false
		}
		for _, ruleChild := range rule.Children {
			found := false
			for _, reqChild := range req.Children {
				if filtersMatch(ruleChild, reqChild) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true

	case FilterNot:
		if len(rule.Children) == 0 || len(req.Children) == 0 {
			return len(rule.Children) == len(req.Children)
		}
		return filtersMatch(rule.Children[0], req.Children[0])

	case FilterEqual, FilterApprox, FilterGreaterOrEqual, FilterLessOrEqual:
		if !strings.EqualFold(rule.Attr, req.Attr) {
			return false
		}
		if strings.Contains(rule.Value, "*") {
			return wildcardMatch(rule.Value, req.Value)
		}
		return strings.EqualFold(rule.Value, req.Value)

	case FilterPresent:
		return strings.EqualFold(rule.Attr, req.Attr)

	case FilterSubstring:
		if !strings.EqualFold(rule.Attr, req.Attr) {
			return false
		}
		if rule.Initial != "" && !strings.EqualFold(rule.Initial, req.Initial) {
			return false
		}
		if rule.Final != "" && !strings.EqualFold(rule.Final, req.Final) {
			return false
		}
		if len(rule.Any) > 0 {
			if len(rule.Any) != len(req.Any) {
				return false
			}
			for i := range rule.Any {
				if !strings.EqualFold(rule.Any[i], req.Any[i]) {
					return false
				}
			}
		}
		return true
	}

	return false
}

func wildcardMatch(pattern, str string) bool {
	pattern = strings.ToLower(pattern)
	str = strings.ToLower(str)

	regexPattern := "^"
	for _, c := range pattern {
		if c == '*' {
			regexPattern += ".*"
		} else if c == '.' || c == '+' || c == '?' || c == '(' || c == ')' ||
			c == '[' || c == ']' || c == '{' || c == '}' || c == '^' || c == '$' || c == '\\' {
			regexPattern += "\\" + string(c)
		} else {
			regexPattern += string(c)
		}
	}
	regexPattern += "$"

	matched, _ := regexp.MatchString(regexPattern, str)

	return matched
}
