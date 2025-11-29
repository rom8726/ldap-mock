package main

import (
	"testing"
)

func TestNewRuleEngine_SortsByPriority(t *testing.T) {
	rules := []Rule{
		{Name: "low", Priority: 1},
		{Name: "high", Priority: 10},
		{Name: "medium", Priority: 5},
	}

	engine := NewRuleEngine(rules)

	if engine.rules[0].Name != "high" {
		t.Errorf("first rule = %v, want high", engine.rules[0].Name)
	}
	if engine.rules[1].Name != "medium" {
		t.Errorf("second rule = %v, want medium", engine.rules[1].Name)
	}
	if engine.rules[2].Name != "low" {
		t.Errorf("third rule = %v, want low", engine.rules[2].Name)
	}
}

func TestNewRuleEngine_DoesNotModifyOriginal(t *testing.T) {
	rules := []Rule{
		{Name: "low", Priority: 1},
		{Name: "high", Priority: 10},
	}

	_ = NewRuleEngine(rules)

	if rules[0].Name != "low" {
		t.Error("original slice was modified")
	}
}

func TestParseScope(t *testing.T) {
	tests := []struct {
		input string
		want  LDAPScope
	}{
		{"base", ScopeBase},
		{"BASE", ScopeBase},
		{"one", ScopeOne},
		{"ONE", ScopeOne},
		{"sub", ScopeSub},
		{"SUB", ScopeSub},
		{"", ScopeSub},
		{"unknown", ScopeSub},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseScope(tt.input)
			if got != tt.want {
				t.Errorf("ParseScope(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLDAPScope_String(t *testing.T) {
	tests := []struct {
		scope LDAPScope
		want  string
	}{
		{ScopeBase, "base"},
		{ScopeOne, "one"},
		{ScopeSub, "sub"},
		{LDAPScope(99), "sub"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.scope.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindMatchingRule_FilterMatch(t *testing.T) {
	rules := []Rule{
		{
			Name:   "john-rule",
			Filter: "(cn=John)",
		},
	}

	engine := NewRuleEngine(rules)

	req := SearchRequest{
		Filter: "(cn=John)",
	}

	rule := engine.FindMatchingRule(req)
	if rule == nil {
		t.Fatal("expected rule, got nil")
	}
	if rule.Name != "john-rule" {
		t.Errorf("rule name = %v, want john-rule", rule.Name)
	}
}

func TestFindMatchingRule_NoMatch(t *testing.T) {
	rules := []Rule{
		{
			Name:   "john-rule",
			Filter: "(cn=John)",
		},
	}

	engine := NewRuleEngine(rules)

	req := SearchRequest{
		Filter: "(cn=Jane)",
	}

	rule := engine.FindMatchingRule(req)
	if rule != nil {
		t.Errorf("expected nil, got %v", rule.Name)
	}
}

func TestFindMatchingRule_BaseDNMatch(t *testing.T) {
	rules := []Rule{
		{
			Name:   "example-rule",
			Filter: "(cn=John)",
			BaseDN: "DC=example,DC=com",
		},
	}

	engine := NewRuleEngine(rules)

	t.Run("matching BaseDN", func(t *testing.T) {
		req := SearchRequest{
			BaseDN: "DC=example,DC=com",
			Filter: "(cn=John)",
		}
		rule := engine.FindMatchingRule(req)
		if rule == nil {
			t.Fatal("expected rule, got nil")
		}
	})

	t.Run("non-matching BaseDN", func(t *testing.T) {
		req := SearchRequest{
			BaseDN: "DC=other,DC=com",
			Filter: "(cn=John)",
		}
		rule := engine.FindMatchingRule(req)
		if rule != nil {
			t.Error("expected nil rule")
		}
	})

	t.Run("case insensitive BaseDN", func(t *testing.T) {
		req := SearchRequest{
			BaseDN: "dc=EXAMPLE,dc=COM",
			Filter: "(cn=John)",
		}
		rule := engine.FindMatchingRule(req)
		if rule == nil {
			t.Fatal("expected rule, got nil")
		}
	})
}

func TestFindMatchingRule_ScopeMatch(t *testing.T) {
	rules := []Rule{
		{
			Name:   "sub-rule",
			Filter: "(cn=John)",
			Scope:  "sub",
		},
	}

	engine := NewRuleEngine(rules)

	t.Run("matching scope", func(t *testing.T) {
		req := SearchRequest{
			Scope:  ScopeSub,
			Filter: "(cn=John)",
		}
		rule := engine.FindMatchingRule(req)
		if rule == nil {
			t.Fatal("expected rule, got nil")
		}
	})

	t.Run("non-matching scope", func(t *testing.T) {
		req := SearchRequest{
			Scope:  ScopeBase,
			Filter: "(cn=John)",
		}
		rule := engine.FindMatchingRule(req)
		if rule != nil {
			t.Error("expected nil rule")
		}
	})
}

func TestFindMatchingRule_PriorityOrder(t *testing.T) {
	rules := []Rule{
		{
			Name:     "low-priority",
			Filter:   "(cn=John)",
			Priority: 1,
		},
		{
			Name:     "high-priority",
			Filter:   "(cn=John)",
			Priority: 10,
		},
	}

	engine := NewRuleEngine(rules)

	req := SearchRequest{
		Filter: "(cn=John)",
	}

	rule := engine.FindMatchingRule(req)
	if rule == nil {
		t.Fatal("expected rule, got nil")
	}
	if rule.Name != "high-priority" {
		t.Errorf("rule name = %v, want high-priority", rule.Name)
	}
}

func TestFindMatchingRule_WildcardFilter(t *testing.T) {
	rules := []Rule{
		{
			Name:   "john-wildcard",
			Filter: "(cn=John*)",
		},
	}

	engine := NewRuleEngine(rules)

	tests := []struct {
		name      string
		reqFilter string
		wantMatch bool
	}{
		{"exact match", "(cn=John)", false},
		{"same pattern", "(cn=John*)", true},
		{"different pattern", "(cn=Jane*)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := SearchRequest{Filter: tt.reqFilter}
			rule := engine.FindMatchingRule(req)
			if tt.wantMatch && rule == nil {
				t.Error("expected match")
			}
			if !tt.wantMatch && rule != nil {
				t.Error("expected no match")
			}
		})
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		str     string
		want    bool
	}{
		{"John*", "JohnDoe", true},
		{"John*", "John", true},
		{"*Doe", "JohnDoe", true},
		{"*Doe", "Doe", true},
		{"J*n", "John", true},
		{"J*n", "Jon", true},
		{"*oh*", "John", true},
		{"John", "John", true},
		{"John", "Jane", false},
		{"John*", "Jane", false},
		{"*Doe", "Smith", false},
		{"J*n*e", "Jane", true},
		{"J*n*e", "Janine", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.str, func(t *testing.T) {
			got := wildcardMatch(tt.pattern, tt.str)
			if got != tt.want {
				t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.str, got, tt.want)
			}
		})
	}
}

func TestFiltersMatch(t *testing.T) {
	tests := []struct {
		name       string
		ruleFilter string
		reqFilter  string
		want       bool
	}{
		{
			name:       "exact equality match",
			ruleFilter: "(cn=John)",
			reqFilter:  "(cn=John)",
			want:       true,
		},
		{
			name:       "case insensitive attr",
			ruleFilter: "(cn=John)",
			reqFilter:  "(CN=John)",
			want:       true,
		},
		{
			name:       "case insensitive value",
			ruleFilter: "(cn=john)",
			reqFilter:  "(cn=JOHN)",
			want:       true,
		},
		{
			name:       "different values",
			ruleFilter: "(cn=John)",
			reqFilter:  "(cn=Jane)",
			want:       false,
		},
		{
			name:       "different attrs",
			ruleFilter: "(cn=John)",
			reqFilter:  "(mail=John)",
			want:       false,
		},
		{
			name:       "and subset match",
			ruleFilter: "(&(cn=John))",
			reqFilter:  "(&(cn=John)(mail=john@example.com))",
			want:       true,
		},
		{
			name:       "and superset no match",
			ruleFilter: "(&(cn=John)(mail=john@example.com))",
			reqFilter:  "(&(cn=John))",
			want:       false,
		},
		{
			name:       "or any child match",
			ruleFilter: "(|(cn=John))",
			reqFilter:  "(cn=John)",
			want:       true,
		},
		{
			name:       "presence filter",
			ruleFilter: "(cn=*)",
			reqFilter:  "(cn=*)",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRuleFilter(tt.ruleFilter, tt.reqFilter)
			if got != tt.want {
				t.Errorf("matchRuleFilter(%q, %q) = %v, want %v", tt.ruleFilter, tt.reqFilter, got, tt.want)
			}
		})
	}
}
