package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	godap "github.com/bradleypeabody/godap"
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
}

func NewLDAPServer(
	log *zap.Logger,
	port string,
	username string,
	password string,
) *LDAPServer {
	s := &LDAPServer{
		port:     port,
		username: username,
		password: password,
		log:      log.Named("ldap_server"),
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

func (s *LDAPServer) getMock() LDAPMock {
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
		s.log.Info("search request")

		mock := s.getMock()
		ret := make([]*godap.LDAPSimpleSearchResultEntry, 0, len(mock.Users))

		for _, user := range mock.Users {
			attrs := make(map[string]any, len(user.Attrs))
			for k, v := range user.Attrs {
				attrs[k] = v
			}

			ret = append(ret, &godap.LDAPSimpleSearchResultEntry{
				DN:    "cn=" + user.CN + "," + req.BaseDN,
				Attrs: attrs,
			})
		}

		return ret
	}})
}
