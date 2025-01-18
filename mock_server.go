package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

type MockHolder interface {
	SetMock(mock LDAPMock)
}

type MockServer struct {
	srv http.Server

	port       string
	log        *zap.Logger
	mockHolder MockHolder
}

func NewMockServer(log *zap.Logger, port string, mockHolder MockHolder) *MockServer {
	s := &MockServer{
		port:       port,
		log:        log.Named("mock_server"),
		mockHolder: mockHolder,
	}

	s.initHandlers()

	return s
}

func (s *MockServer) ListenAndServe(ctx context.Context) error {
	lis, err := net.Listen("tcp", net.JoinHostPort("", s.port))
	if err != nil {
		return fmt.Errorf("listen http: %w", err)
	}

	go func() {
		err := s.srv.Serve(lis)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(fmt.Errorf("HTTP serve: %v", err))
		}
	}()

	s.log.Info("server started")
	<-ctx.Done()
	s.log.Info("shutdown...")

	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	return s.srv.Shutdown(ctxTimeout)
}

func (s *MockServer) initHandlers() {
	router := httprouter.New()

	router.POST("/mock", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		s.log.Info("mock request")

		defer func() { _ = r.Body.Close() }()

		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(fmt.Sprintf("read body: %v", err)))
			return
		}

		var mock LDAPMock
		if err := yaml.Unmarshal(data, &mock); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(fmt.Sprintf("decode mock: %v", err)))
			return
		}

		s.mockHolder.SetMock(mock)
	})

	router.POST("/clean", func(http.ResponseWriter, *http.Request, httprouter.Params) {
		s.log.Info("clean request")
		s.mockHolder.SetMock(LDAPMock{})
	})

	s.srv.Handler = router
}
