package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type testServer struct {
	ldapPort string
	mockPort string
	cancel   context.CancelFunc
	done     chan struct{}
}

func startTestServer(t *testing.T, username, password string) *testServer {
	t.Helper()

	ldapPort := getFreePort(t)
	mockPort := getFreePort(t)

	log, _ := zap.NewDevelopment()
	requestLogger := NewInMemoryRequestLogger(DefaultRequestLogCapacity)

	ctx, cancel := context.WithCancel(context.Background())

	ldapSrv := NewLDAPServer(log, ldapPort, username, password, requestLogger)
	mockSrv := NewMockServer(log, mockPort, ldapSrv, requestLogger)

	done := make(chan struct{})

	go func() {
		group, groupCtx := errgroup.WithContext(ctx)
		group.Go(func() error { return ldapSrv.ListenAndServe(groupCtx) })
		group.Go(func() error { return mockSrv.ListenAndServe(groupCtx) })
		_ = group.Wait()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	return &testServer{
		ldapPort: ldapPort,
		mockPort: mockPort,
		cancel:   cancel,
		done:     done,
	}
}

func (s *testServer) stop() {
	s.cancel()
	<-s.done
}

func (s *testServer) setMock(t *testing.T, yaml string) {
	t.Helper()

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%s/mock", s.mockPort),
		"application/yaml",
		bytes.NewBufferString(yaml),
	)
	if err != nil {
		t.Fatalf("set mock: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set mock: status %d", resp.StatusCode)
	}
}

func (s *testServer) clean(t *testing.T) {
	t.Helper()

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%s/clean", s.mockPort),
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	defer resp.Body.Close()
}

func (s *testServer) ldapDial(t *testing.T) *ldap.Conn {
	t.Helper()

	conn, err := ldap.DialURL(fmt.Sprintf("ldap://localhost:%s", s.ldapPort))
	if err != nil {
		t.Fatalf("ldap dial: %v", err)
	}

	return conn
}

func getFreePort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	defer l.Close()

	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func TestIntegration_Bind(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	t.Run("valid credentials", func(t *testing.T) {
		conn := srv.ldapDial(t)
		defer conn.Close()

		err := conn.Bind("cn=admin", "secret")
		if err != nil {
			t.Errorf("bind failed: %v", err)
		}
	})

	t.Run("invalid credentials", func(t *testing.T) {
		conn := srv.ldapDial(t)
		defer conn.Close()

		err := conn.Bind("cn=admin", "wrong")
		if err == nil {
			t.Error("expected bind to fail")
		}
	})
}

func TestIntegration_SearchFallback(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
users:
  - cn: john.doe
    attrs:
      mail: john@example.com
      title: Developer
  - cn: jane.doe
    attrs:
      mail: jane@example.com
      title: Manager
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	result, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     "DC=example,DC=com",
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     "(objectClass=*)",
		Attributes: []string{"mail", "title"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(result.Entries))
	}
}

func TestIntegration_SearchWithFilter(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
users:
  - cn: john.doe
    attrs:
      mail: john@example.com
  - cn: jane.doe
    attrs:
      mail: jane@example.com
  - cn: bob.smith
    attrs:
      mail: bob@other.com
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	t.Run("filter by exact mail", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=example,DC=com",
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     "(mail=bob@other.com)",
			Attributes: []string{"mail"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Errorf("entries = %d, want 1", len(result.Entries))
		}
	})

	t.Run("filter by cn", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=example,DC=com",
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     "(cn=john.doe)",
			Attributes: []string{"mail"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Errorf("entries = %d, want 1", len(result.Entries))
		}

		mail := result.Entries[0].GetAttributeValue("mail")
		if mail != "john@example.com" {
			t.Errorf("mail = %q, want john@example.com", mail)
		}
	})
}

func TestIntegration_RulesBasic(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
users:
  - cn: fallback-user
    attrs:
      mail: fallback@example.com

rules:
  - name: john-rule
    filter: "(cn=john)"
    response:
      users:
        - cn: john.doe
          attrs:
            mail: john@example.com
            title: Developer
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	t.Run("matching rule", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=example,DC=com",
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     "(cn=john)",
			Attributes: []string{"mail", "title"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(result.Entries))
		}

		mail := result.Entries[0].GetAttributeValue("mail")
		if mail != "john@example.com" {
			t.Errorf("mail = %q, want john@example.com", mail)
		}
	})

	t.Run("fallback when no rule matches", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=example,DC=com",
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     "(cn=fallback-user)",
			Attributes: []string{"mail"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Errorf("entries = %d, want 1 (fallback user)", len(result.Entries))
		}

		mail := result.Entries[0].GetAttributeValue("mail")
		if mail != "fallback@example.com" {
			t.Errorf("mail = %q, want fallback@example.com", mail)
		}
	})
}

func TestIntegration_RulesPriority(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
rules:
  - name: low-priority
    filter: "(cn=test)"
    priority: 1
    response:
      users:
        - cn: low
          attrs:
            source: low-priority

  - name: high-priority
    filter: "(cn=test)"
    priority: 10
    response:
      users:
        - cn: high
          attrs:
            source: high-priority
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	result, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     "DC=example,DC=com",
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     "(cn=test)",
		Attributes: []string{"source"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(result.Entries))
	}

	source := result.Entries[0].GetAttributeValue("source")
	if source != "high-priority" {
		t.Errorf("source = %q, want high-priority", source)
	}
}

func TestIntegration_RulesBaseDN(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
rules:
  - name: example-rule
    filter: "(cn=user)"
    base_dn: "DC=example,DC=com"
    response:
      users:
        - cn: example-user
          attrs:
            domain: example

  - name: other-rule
    filter: "(cn=user)"
    base_dn: "DC=other,DC=com"
    response:
      users:
        - cn: other-user
          attrs:
            domain: other
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	t.Run("example.com base", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=example,DC=com",
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     "(cn=user)",
			Attributes: []string{"domain"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(result.Entries))
		}

		domain := result.Entries[0].GetAttributeValue("domain")
		if domain != "example" {
			t.Errorf("domain = %q, want example", domain)
		}
	})

	t.Run("other.com base", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=other,DC=com",
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     "(cn=user)",
			Attributes: []string{"domain"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(result.Entries))
		}

		domain := result.Entries[0].GetAttributeValue("domain")
		if domain != "other" {
			t.Errorf("domain = %q, want other", domain)
		}
	})
}

func TestIntegration_RulesScope(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
rules:
  - name: sub-rule
    filter: "(cn=user)"
    scope: "sub"
    response:
      users:
        - cn: sub-user
          attrs:
            scope: sub

  - name: base-rule
    filter: "(cn=user)"
    scope: "base"
    response:
      users:
        - cn: base-user
          attrs:
            scope: base
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	t.Run("subtree scope", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=example,DC=com",
			Scope:      ldap.ScopeWholeSubtree,
			Filter:     "(cn=user)",
			Attributes: []string{"scope"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(result.Entries))
		}

		scope := result.Entries[0].GetAttributeValue("scope")
		if scope != "sub" {
			t.Errorf("scope = %q, want sub", scope)
		}
	})

	t.Run("base scope", func(t *testing.T) {
		result, err := conn.Search(&ldap.SearchRequest{
			BaseDN:     "DC=example,DC=com",
			Scope:      ldap.ScopeBaseObject,
			Filter:     "(cn=user)",
			Attributes: []string{"scope"},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		if len(result.Entries) != 1 {
			t.Fatalf("entries = %d, want 1", len(result.Entries))
		}

		scope := result.Entries[0].GetAttributeValue("scope")
		if scope != "base" {
			t.Errorf("scope = %q, want base", scope)
		}
	})
}

func TestIntegration_RulesMultipleUsers(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
rules:
  - name: team-rule
    filter: "(team=backend)"
    response:
      users:
        - cn: john.doe
          attrs:
            mail: john.doe@example.com
            team: backend
        - cn: jane.doe
          attrs:
            mail: jane.doe@example.com
            team: backend
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	result, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     "DC=example,DC=com",
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     "(team=backend)",
		Attributes: []string{"mail"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(result.Entries))
	}
}

func TestIntegration_Clean(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
users:
  - cn: test
    attrs:
      mail: test@example.com
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	result, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     "DC=example,DC=com",
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     "(cn=test)",
		Attributes: []string{"mail"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("entries before clean = %d, want 1", len(result.Entries))
	}

	srv.clean(t)

	srv.setMock(t, `
users:
  - cn: new-user
    attrs:
      mail: new@example.com
`)

	result, err = conn.Search(&ldap.SearchRequest{
		BaseDN:     "DC=example,DC=com",
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     "(cn=new-user)",
		Attributes: []string{"mail"},
	})
	if err != nil {
		t.Fatalf("search after clean and new mock: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Errorf("entries after clean = %d, want 1", len(result.Entries))
	}

	mail := result.Entries[0].GetAttributeValue("mail")
	if mail != "new@example.com" {
		t.Errorf("mail = %q, want new@example.com", mail)
	}
}

func TestIntegration_MultipleRulesResponse(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
rules:
  - name: developers
    filter: "(department=engineering)"
    response:
      users:
        - cn: dev1
          attrs:
            department: engineering
            role: developer
        - cn: dev2
          attrs:
            department: engineering
            role: developer
        - cn: lead
          attrs:
            department: engineering
            role: lead
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	err := conn.Bind("cn=admin", "secret")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	result, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     "DC=example,DC=com",
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     "(department=engineering)",
		Attributes: []string{"role"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(result.Entries) != 3 {
		t.Errorf("entries = %d, want 3", len(result.Entries))
	}
}

func TestIntegration_RequestLogAPI(t *testing.T) {
	srv := startTestServer(t, "cn=admin", "secret")
	defer srv.stop()

	srv.setMock(t, `
users:
  - cn: john.doe
    attrs:
      mail: john@example.com
`)

	conn := srv.ldapDial(t)
	defer conn.Close()

	if err := conn.Bind("cn=admin", "secret"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	_, err := conn.Search(&ldap.SearchRequest{
		BaseDN:     "DC=example,DC=com",
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     "(cn=john.doe)",
		Attributes: []string{"mail"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	t.Run("list with limit", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/requests?limit=1", srv.mockPort))
		if err != nil {
			t.Fatalf("get requests: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var logs []LDAPRequestLog
		if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if len(logs) != 1 {
			t.Fatalf("logs length = %d, want 1", len(logs))
		}
		if logs[0].Filter != "(cn=john.doe)" {
			t.Fatalf("filter = %q, want (cn=john.doe)", logs[0].Filter)
		}
		if logs[0].Response.Count != 1 {
			t.Fatalf("response count = %d, want 1", logs[0].Response.Count)
		}
	})

	t.Run("clear", func(t *testing.T) {
		resp, err := http.Post(fmt.Sprintf("http://localhost:%s/requests/clear", srv.mockPort), "", nil)
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		resp.Body.Close()

		resp, err = http.Get(fmt.Sprintf("http://localhost:%s/requests", srv.mockPort))
		if err != nil {
			t.Fatalf("get requests after clear: %v", err)
		}
		defer resp.Body.Close()

		var logs []LDAPRequestLog
		if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
			t.Fatalf("decode after clear: %v", err)
		}

		if len(logs) != 0 {
			t.Fatalf("logs length after clear = %d, want 0", len(logs))
		}
	})
}
