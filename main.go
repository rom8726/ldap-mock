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
		_, _ = fmt.Fprintf(os.Stderr, err.Error()+"\n")
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

	ldapSrv := NewLDAPServer(
		log,
		os.Getenv("LDAP_PORT"),
		os.Getenv("LDAP_USERNAME"),
		os.Getenv("LDAP_PASSWORD"),
	)

	mockSrv := NewMockServer(log, os.Getenv("MOCK_PORT"), ldapSrv)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return ldapSrv.ListenAndServe(groupCtx) })
	group.Go(func() error { return mockSrv.ListenAndServe(groupCtx) })

	return group.Wait()
}
