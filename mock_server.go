package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

type MockHolder interface {
	SetMock(mock LDAPMock)
	GetMock() LDAPMock
}

type MockServer struct {
	srv http.Server

	port          string
	log           *zap.Logger
	mockHolder    MockHolder
	requestLogger RequestLogger
	mockMu        sync.RWMutex
	lastMockYAML  string
}

func NewMockServer(log *zap.Logger, port string, mockHolder MockHolder, requestLogger RequestLogger) *MockServer {
	if requestLogger == nil {
		requestLogger = NewInMemoryRequestLogger(DefaultRequestLogCapacity)
	}

	s := &MockServer{
		port:          port,
		log:           log.Named("mock_server"),
		mockHolder:    mockHolder,
		requestLogger: requestLogger,
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

		s.mockMu.Lock()
		s.lastMockYAML = string(data)
		s.mockMu.Unlock()

		w.WriteHeader(http.StatusOK)
	})

	router.POST("/clean", func(http.ResponseWriter, *http.Request, httprouter.Params) {
		s.log.Info("clean request")
		s.mockHolder.SetMock(LDAPMock{})
	})

	router.GET("/requests", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		limitParam := r.URL.Query().Get("limit")
		limit := -1
		if limitParam != "" {
			val, err := strconv.Atoi(limitParam)
			if err != nil || val < 0 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("invalid limit"))
				return
			}
			limit = val
		}

		logs := s.requestLogger.List()
		if limit >= 0 && len(logs) > limit {
			logs = logs[:limit]
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(logs); err != nil {
			s.log.Warn("encode requests", zap.Error(err))
		}
	})

	router.POST("/requests/clear", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		s.log.Info("requests clear")
		s.requestLogger.Clear()
		w.WriteHeader(http.StatusOK)
	})

	router.GET("/mock", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		mock := s.mockHolder.GetMock()

		s.mockMu.RLock()
		yamlData := s.lastMockYAML
		s.mockMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		resp := struct {
			Mock LDAPMock `json:"mock"`
			YAML string   `json:"yaml"`
		}{
			Mock: mock,
			YAML: yamlData,
		}

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(resp); err != nil {
			s.log.Warn("encode mock", zap.Error(err))
		}
	})

	router.GET("/ui", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(uiIndexHTML))
	})

	router.GET("/ui/*path", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(uiIndexHTML))
	})

	s.srv.Handler = router
}
