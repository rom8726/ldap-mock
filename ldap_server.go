package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	godap "github.com/bradleypeabody/godap"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type LDAPServer struct {
	srv      godap.LDAPServer
	port     string
	username string
	password string
	log      *zap.Logger

	usersMock LDAPMock
	mu        sync.Mutex

	requestLogger RequestLogger
}

func NewLDAPServer(
	log *zap.Logger,
	port string,
	username string,
	password string,
	requestLogger RequestLogger,
) *LDAPServer {
	if requestLogger == nil {
		requestLogger = NewInMemoryRequestLogger(DefaultRequestLogCapacity)
	}

	s := &LDAPServer{
		port:          port,
		username:      username,
		password:      password,
		log:           log.Named("ldap_server"),
		requestLogger: requestLogger,
	}

	s.initHandlers()

	return s
}

func (s *LDAPServer) ListenAndServe(ctx context.Context) error {
	lis, err := net.Listen("tcp", net.JoinHostPort("", s.port))
	if err != nil {
		return fmt.Errorf("listen LDAP: %w", err)
	}

	s.srv.Listener = lis

	go func() {
		err := s.srv.Serve()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			panic(fmt.Errorf("LDAP serve: %v", err))
		}
	}()

	s.log.Info("server started")
	<-ctx.Done()
	s.log.Info("shutdown...")

	return s.srv.Listener.Close()
}

func (s *LDAPServer) SetMock(mock LDAPMock) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.usersMock = mock
}

func (s *LDAPServer) GetMock() LDAPMock {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.usersMock
}

func (s *LDAPServer) initHandlers() {
	s.srv.Handlers = append(s.srv.Handlers, &godap.LDAPBindFuncHandler{LDAPBindFunc: func(binddn string, bindpw []byte) bool {
		s.log.Info("bind attempt")

		if binddn == s.username && string(bindpw) == s.password {
			s.log.Info("binded")

			return true
		}

		s.log.Info("bind: invalid creds")

		return false
	}})

	s.srv.Handlers = append(s.srv.Handlers, &godap.LDAPSimpleSearchFuncHandler{LDAPSimpleSearchFunc: func(req *godap.LDAPSimpleSearchRequest) []*godap.LDAPSimpleSearchResultEntry {
		s.log.Info("search request",
			zap.String("base_dn", req.BaseDN),
			zap.String("filter_attr", req.FilterAttr),
			zap.String("filter_value", req.FilterValue),
			zap.Int64("scope", req.Scope),
		)

		filter := buildFilter(req.FilterAttr, req.FilterValue)

		mock := s.GetMock()

		users, groups, matchedRule := s.findMatchingEntries(mock, req, filter)

		ret := make([]*godap.LDAPSimpleSearchResultEntry, 0, len(users)+len(groups))
		returnedDNs := make([]string, 0, len(users)+len(groups))

		for _, user := range users {
			attrs := make(map[string]any, len(user.Attrs))
			for k, v := range user.Attrs {
				attrs[k] = v
			}

			returnedDNs = append(returnedDNs, user.CN)

			ret = append(ret, &godap.LDAPSimpleSearchResultEntry{
				DN:    user.CN,
				Attrs: attrs,
			})
		}

		for _, group := range groups {
			attrs := make(map[string]any, len(group.Attrs)+1)
			for k, v := range group.Attrs {
				attrs[k] = v
			}
			if len(group.Members) > 0 {
				attrs["member"] = group.Members
			}

			returnedDNs = append(returnedDNs, group.CN)

			ret = append(ret, &godap.LDAPSimpleSearchResultEntry{
				DN:    group.CN,
				Attrs: attrs,
			})
		}

		requestLog := LDAPRequestLog{
			Timestamp:  time.Now().UTC(),
			RequestID:  uuid.NewString(),
			Type:       "search",
			BaseDN:     req.BaseDN,
			Scope:      LDAPScope(req.Scope).String(),
			Filter:     filter,
			Attributes: nil,
			Response: LDAPResponseLog{
				ReturnedDNs: returnedDNs,
				Count:       len(returnedDNs),
			},
		}

		if matchedRule != nil {
			requestLog.MatchedRule = &MatchedRuleLog{
				RuleID:   matchedRule.ID,
				RuleName: matchedRule.Name,
			}
		}

		s.requestLogger.Log(requestLog)

		return ret
	}})
}

func (s *LDAPServer) findMatchingEntries(mock LDAPMock, req *godap.LDAPSimpleSearchRequest, filter string) ([]User, []Group, *Rule) {
	if len(mock.Rules) > 0 {
		engine := NewRuleEngine(mock.Rules)

		searchReq := SearchRequest{
			BaseDN: req.BaseDN,
			Scope:  LDAPScope(req.Scope),
			Filter: filter,
		}

		if rule := engine.FindMatchingRule(searchReq); rule != nil {
			s.log.Info("rule matched", zap.String("rule", rule.Name))
			return rule.Response.Users, rule.Response.Groups, rule
		}
	}

	return filterUsers(mock.Users, filter), nil, nil
}

func (s *LDAPServer) RequestLogger() RequestLogger {
	return s.requestLogger
}

func filterUsers(users []User, filterStr string) []User {
	if filterStr == "(objectClass=*)" || filterStr == "" {
		return users
	}

	filter, err := ParseFilter(filterStr)
	if err != nil {
		return users
	}

	result := make([]User, 0, len(users))
	for _, user := range users {
		attrs := make(map[string]string, len(user.Attrs)+1)
		attrs["cn"] = user.CN
		for k, v := range user.Attrs {
			attrs[k] = v
		}

		if MatchFilter(filter, attrs) {
			result = append(result, user)
		}
	}

	return result
}

func buildFilter(attr, value string) string {
	if attr == "" || attr == "searchFingerprint" {
		return "(objectClass=*)"
	}

	return "(" + attr + "=" + value + ")"
}
