package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run() error {
	log, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	defer func() { _ = log.Sync() }()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	requestLogger := NewInMemoryRequestLogger(DefaultRequestLogCapacity)

	ldapSrv := NewLDAPServer(
		log,
		getLDAPPort(),
		os.Getenv("LDAP_USERNAME"),
		os.Getenv("LDAP_PASSWORD"),
		requestLogger,
	)

	mockSrv := NewMockServer(log, getMockPort(), ldapSrv)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return ldapSrv.ListenAndServe(groupCtx) })
	group.Go(func() error { return mockSrv.ListenAndServe(groupCtx) })

	return group.Wait()
}

func getLDAPPort() string {
	envPort := os.Getenv("LDAP_PORT")
	if envPort == "" {
		return "389"
	}

	return envPort
}

func getMockPort() string {
	envPort := os.Getenv("MOCK_PORT")
	if envPort == "" {
		return "6006"
	}

	return envPort
}
