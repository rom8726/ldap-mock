package main

import (
	"fmt"
	"strings"
)

type FilterType int

const (
	FilterAnd FilterType = iota
	FilterOr
	FilterNot
	FilterEqual
	FilterApprox
	FilterGreaterOrEqual
	FilterLessOrEqual
	FilterPresent
	FilterSubstring
)

type Filter struct {
	Type     FilterType
	Attr     string
	Value    string
	Children []*Filter
	Initial  string
	Any      []string
	Final    string
}

func ParseFilter(filterStr string) (*Filter, error) {
	filterStr = strings.TrimSpace(filterStr)
	if len(filterStr) == 0 {
		return nil, fmt.Errorf("empty filter")
	}

	if filterStr[0] != '(' || filterStr[len(filterStr)-1] != ')' {
		return nil, fmt.Errorf("filter must be enclosed in parentheses")
	}

	return parseFilterComp(filterStr[1 : len(filterStr)-1])
}

func parseFilterComp(s string) (*Filter, error) {
	if len(s) == 0 {
		return nil, fmt.Errorf("empty filter component")
	}

	switch s[0] {
	case '&':
		return parseFilterList(s[1:], FilterAnd)
	case '|':
		return parseFilterList(s[1:], FilterOr)
	case '!':
		child, err := ParseFilter(s[1:])
		if err != nil {
			return nil, fmt.Errorf("parse NOT filter: %w", err)
		}
		return &Filter{Type: FilterNot, Children: []*Filter{child}}, nil
	default:
		return parseItem(s)
	}
}

func parseFilterList(s string, filterType FilterType) (*Filter, error) {
	filter := &Filter{Type: filterType}

	for len(s) > 0 {
		if s[0] != '(' {
			return nil, fmt.Errorf("expected '(' in filter list, got %c", s[0])
		}

		depth := 0
		end := 0
		for i, c := range s {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}

		if depth != 0 {
			return nil, fmt.Errorf("unbalanced parentheses in filter list")
		}

		child, err := ParseFilter(s[:end+1])
		if err != nil {
			return nil, err
		}
		filter.Children = append(filter.Children, child)
		s = s[end+1:]
	}

	if len(filter.Children) == 0 {
		return nil, fmt.Errorf("empty filter list")
	}

	return filter, nil
}

func parseItem(s string) (*Filter, error) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '=':
			attr := s[:i]
			value := s[i+1:]

			if i > 0 && s[i-1] == '~' {
				return &Filter{
					Type:  FilterApprox,
					Attr:  strings.ToLower(s[:i-1]),
					Value: value,
				}, nil
			}

			if i > 0 && s[i-1] == '>' {
				return &Filter{
					Type:  FilterGreaterOrEqual,
					Attr:  strings.ToLower(s[:i-1]),
					Value: value,
				}, nil
			}

			if i > 0 && s[i-1] == '<' {
				return &Filter{
					Type:  FilterLessOrEqual,
					Attr:  strings.ToLower(s[:i-1]),
					Value: value,
				}, nil
			}

			if value == "*" {
				return &Filter{
					Type: FilterPresent,
					Attr: strings.ToLower(attr),
				}, nil
			}

			if strings.Contains(value, "*") {
				return parseSubstringFilter(attr, value)
			}

			return &Filter{
				Type:  FilterEqual,
				Attr:  strings.ToLower(attr),
				Value: value,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid filter item: %s", s)
}

func parseSubstringFilter(attr, value string) (*Filter, error) {
	filter := &Filter{
		Type: FilterSubstring,
		Attr: strings.ToLower(attr),
	}

	parts := strings.Split(value, "*")

	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid substring filter")
	}

	if parts[0] != "" {
		filter.Initial = parts[0]
	}

	if len(parts) > 1 && parts[len(parts)-1] != "" {
		filter.Final = parts[len(parts)-1]
	}

	for i := 1; i < len(parts)-1; i++ {
		if parts[i] != "" {
			filter.Any = append(filter.Any, parts[i])
		}
	}

	return filter, nil
}

func MatchFilter(filter *Filter, attrs map[string]string) bool {
	normalizedAttrs := make(map[string]string, len(attrs))
	for k, v := range attrs {
		normalizedAttrs[strings.ToLower(k)] = v
	}

	return matchFilterInternal(filter, normalizedAttrs)
}

func matchFilterInternal(filter *Filter, attrs map[string]string) bool {
	switch filter.Type {
	case FilterAnd:
		for _, child := range filter.Children {
			if !matchFilterInternal(child, attrs) {
				return false
			}
		}
		return true

	case FilterOr:
		for _, child := range filter.Children {
			if matchFilterInternal(child, attrs) {
				return true
			}
		}
		return false

	case FilterNot:
		if len(filter.Children) == 0 {
			return true
		}
		return !matchFilterInternal(filter.Children[0], attrs)

	case FilterEqual:
		val, ok := attrs[filter.Attr]
		if !ok {
			return false
		}
		return strings.EqualFold(val, filter.Value)

	case FilterApprox:
		val, ok := attrs[filter.Attr]
		if !ok {
			return false
		}
		return strings.EqualFold(val, filter.Value)

	case FilterGreaterOrEqual:
		val, ok := attrs[filter.Attr]
		if !ok {
			return false
		}
		return strings.ToLower(val) >= strings.ToLower(filter.Value)

	case FilterLessOrEqual:
		val, ok := attrs[filter.Attr]
		if !ok {
			return false
		}
		return strings.ToLower(val) <= strings.ToLower(filter.Value)

	case FilterPresent:
		_, ok := attrs[filter.Attr]
		return ok

	case FilterSubstring:
		val, ok := attrs[filter.Attr]
		if !ok {
			return false
		}
		return matchSubstring(val, filter.Initial, filter.Any, filter.Final)
	}

	return false
}

func matchSubstring(value, initial string, any []string, final string) bool {
	value = strings.ToLower(value)
	initial = strings.ToLower(initial)
	final = strings.ToLower(final)

	if initial != "" {
		if !strings.HasPrefix(value, initial) {
			return false
		}
		value = value[len(initial):]
	}

	if final != "" {
		if !strings.HasSuffix(value, final) {
			return false
		}
		value = value[:len(value)-len(final)]
	}

	for _, part := range any {
		part = strings.ToLower(part)
		idx := strings.Index(value, part)
		if idx == -1 {
			return false
		}
		value = value[idx+len(part):]
	}

	return true
}
