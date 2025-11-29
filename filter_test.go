package main

import (
	"testing"
)

func TestParseFilter_Simple(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantType  FilterType
		wantAttr  string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "equality",
			input:     "(cn=John)",
			wantType:  FilterEqual,
			wantAttr:  "cn",
			wantValue: "John",
		},
		{
			name:      "equality case insensitive attr",
			input:     "(CN=John)",
			wantType:  FilterEqual,
			wantAttr:  "cn",
			wantValue: "John",
		},
		{
			name:     "presence",
			input:    "(mail=*)",
			wantType: FilterPresent,
			wantAttr: "mail",
		},
		{
			name:      "approx",
			input:     "(cn~=John)",
			wantType:  FilterApprox,
			wantAttr:  "cn",
			wantValue: "John",
		},
		{
			name:      "greater or equal",
			input:     "(age>=18)",
			wantType:  FilterGreaterOrEqual,
			wantAttr:  "age",
			wantValue: "18",
		},
		{
			name:      "less or equal",
			input:     "(age<=65)",
			wantType:  FilterLessOrEqual,
			wantAttr:  "age",
			wantValue: "65",
		},
		{
			name:    "empty filter",
			input:   "",
			wantErr: true,
		},
		{
			name:    "no parentheses",
			input:   "cn=John",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFilter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Type != tt.wantType {
				t.Errorf("type = %v, want %v", f.Type, tt.wantType)
			}
			if f.Attr != tt.wantAttr {
				t.Errorf("attr = %v, want %v", f.Attr, tt.wantAttr)
			}
			if f.Value != tt.wantValue {
				t.Errorf("value = %v, want %v", f.Value, tt.wantValue)
			}
		})
	}
}

func TestParseFilter_Substring(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantInitial string
		wantAny     []string
		wantFinal   string
	}{
		{
			name:        "prefix",
			input:       "(cn=John*)",
			wantInitial: "John",
		},
		{
			name:      "suffix",
			input:     "(cn=*Doe)",
			wantFinal: "Doe",
		},
		{
			name:        "prefix and suffix",
			input:       "(cn=J*n)",
			wantInitial: "J",
			wantFinal:   "n",
		},
		{
			name:    "contains",
			input:   "(cn=*oh*)",
			wantAny: []string{"oh"},
		},
		{
			name:        "complex",
			input:       "(cn=J*o*h*n)",
			wantInitial: "J",
			wantAny:     []string{"o", "h"},
			wantFinal:   "n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFilter(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Type != FilterSubstring {
				t.Errorf("type = %v, want FilterSubstring", f.Type)
			}
			if f.Initial != tt.wantInitial {
				t.Errorf("initial = %v, want %v", f.Initial, tt.wantInitial)
			}
			if f.Final != tt.wantFinal {
				t.Errorf("final = %v, want %v", f.Final, tt.wantFinal)
			}
			if len(f.Any) != len(tt.wantAny) {
				t.Errorf("any = %v, want %v", f.Any, tt.wantAny)
			}
			for i := range tt.wantAny {
				if f.Any[i] != tt.wantAny[i] {
					t.Errorf("any[%d] = %v, want %v", i, f.Any[i], tt.wantAny[i])
				}
			}
		})
	}
}

func TestParseFilter_Composite(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantType     FilterType
		wantChildren int
	}{
		{
			name:         "and",
			input:        "(&(cn=John)(mail=john@example.com))",
			wantType:     FilterAnd,
			wantChildren: 2,
		},
		{
			name:         "or",
			input:        "(|(cn=John)(cn=Jane))",
			wantType:     FilterOr,
			wantChildren: 2,
		},
		{
			name:         "not",
			input:        "(!(cn=John))",
			wantType:     FilterNot,
			wantChildren: 1,
		},
		{
			name:         "complex and",
			input:        "(&(cn=John)(mail=*)(title=Developer))",
			wantType:     FilterAnd,
			wantChildren: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFilter(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Type != tt.wantType {
				t.Errorf("type = %v, want %v", f.Type, tt.wantType)
			}
			if len(f.Children) != tt.wantChildren {
				t.Errorf("children count = %v, want %v", len(f.Children), tt.wantChildren)
			}
		})
	}
}

func TestMatchFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		attrs  map[string]string
		want   bool
	}{
		{
			name:   "equality match",
			filter: "(cn=John)",
			attrs:  map[string]string{"cn": "John"},
			want:   true,
		},
		{
			name:   "equality no match",
			filter: "(cn=John)",
			attrs:  map[string]string{"cn": "Jane"},
			want:   false,
		},
		{
			name:   "equality case insensitive",
			filter: "(cn=john)",
			attrs:  map[string]string{"CN": "JOHN"},
			want:   true,
		},
		{
			name:   "presence match",
			filter: "(mail=*)",
			attrs:  map[string]string{"mail": "john@example.com"},
			want:   true,
		},
		{
			name:   "presence no match",
			filter: "(mail=*)",
			attrs:  map[string]string{"cn": "John"},
			want:   false,
		},
		{
			name:   "substring prefix match",
			filter: "(cn=Jo*)",
			attrs:  map[string]string{"cn": "John"},
			want:   true,
		},
		{
			name:   "substring prefix no match",
			filter: "(cn=Ja*)",
			attrs:  map[string]string{"cn": "John"},
			want:   false,
		},
		{
			name:   "substring suffix match",
			filter: "(cn=*ohn)",
			attrs:  map[string]string{"cn": "John"},
			want:   true,
		},
		{
			name:   "substring contains match",
			filter: "(cn=*oh*)",
			attrs:  map[string]string{"cn": "John"},
			want:   true,
		},
		{
			name:   "and match",
			filter: "(&(cn=John)(mail=john@example.com))",
			attrs:  map[string]string{"cn": "John", "mail": "john@example.com"},
			want:   true,
		},
		{
			name:   "and partial no match",
			filter: "(&(cn=John)(mail=jane@example.com))",
			attrs:  map[string]string{"cn": "John", "mail": "john@example.com"},
			want:   false,
		},
		{
			name:   "or first match",
			filter: "(|(cn=John)(cn=Jane))",
			attrs:  map[string]string{"cn": "John"},
			want:   true,
		},
		{
			name:   "or second match",
			filter: "(|(cn=John)(cn=Jane))",
			attrs:  map[string]string{"cn": "Jane"},
			want:   true,
		},
		{
			name:   "or no match",
			filter: "(|(cn=John)(cn=Jane))",
			attrs:  map[string]string{"cn": "Bob"},
			want:   false,
		},
		{
			name:   "not match",
			filter: "(!(cn=John))",
			attrs:  map[string]string{"cn": "Jane"},
			want:   true,
		},
		{
			name:   "not no match",
			filter: "(!(cn=John))",
			attrs:  map[string]string{"cn": "John"},
			want:   false,
		},
		{
			name:   "greater or equal match",
			filter: "(age>=18)",
			attrs:  map[string]string{"age": "25"},
			want:   true,
		},
		{
			name:   "greater or equal exact",
			filter: "(age>=18)",
			attrs:  map[string]string{"age": "18"},
			want:   true,
		},
		{
			name:   "less or equal match",
			filter: "(age<=65)",
			attrs:  map[string]string{"age": "25"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseFilter(tt.filter)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got := MatchFilter(f, tt.attrs)
			if got != tt.want {
				t.Errorf("MatchFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchSubstring(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		initial string
		any     []string
		final   string
		want    bool
	}{
		{
			name:    "prefix match",
			value:   "JohnDoe",
			initial: "John",
			want:    true,
		},
		{
			name:  "suffix match",
			value: "JohnDoe",
			final: "Doe",
			want:  true,
		},
		{
			name:    "prefix and suffix match",
			value:   "JohnDoe",
			initial: "John",
			final:   "Doe",
			want:    true,
		},
		{
			name:  "contains match",
			value: "JohnDoe",
			any:   []string{"ohn"},
			want:  true,
		},
		{
			name:    "complex match",
			value:   "JohnMiddleDoe",
			initial: "John",
			any:     []string{"Middle"},
			final:   "Doe",
			want:    true,
		},
		{
			name:    "prefix no match",
			value:   "JaneDoe",
			initial: "John",
			want:    false,
		},
		{
			name:  "case insensitive",
			value: "JOHNDOE",
			any:   []string{"ohnd"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchSubstring(tt.value, tt.initial, tt.any, tt.final)
			if got != tt.want {
				t.Errorf("matchSubstring() = %v, want %v", got, tt.want)
			}
		})
	}
}
