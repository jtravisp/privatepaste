// Command server runs the PrivatePaste HTTP server.
//
// Lifecycle, in the order Kubernetes drives it:
//  1. Start: load config, build the AWS client (credentials from EKS Pod
//     Identity in the cluster, a shared profile locally), start listening.
//  2. Serve: /readyz reports ready once the table is reachable.
//  3. SIGTERM: mark not-ready, keep serving for ShutdownDelay so the pod is
//     removed from Service endpoints, then drain in-flight requests and exit.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/joho/godotenv"
	app "github.com/jtravisp/privatepaste"
	"github.com/jtravisp/privatepaste/internal/config"
	"github.com/jtravisp/privatepaste/internal/httpserver"
	"github.com/jtravisp/privatepaste/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// .env is for `go run` on a laptop; absent in the container, and that is
	// not an error.
	_ = godotenv.Load()
	cfg := config.Load()
	slog.Info("starting", "env", cfg.Env, "table", cfg.TableName, "region", cfg.Region,
		"local_dynamo", cfg.DynamoEndpoint != "")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}
	if cfg.AWSProfile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(cfg.AWSProfile))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return err
	}
	client := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.DynamoEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.DynamoEndpoint)
		}
	})
	st := store.NewDynamo(client, cfg.TableName)

	if cfg.DynamoEndpoint != "" {
		if err := st.EnsureTable(ctx, 60*time.Second); err != nil {
			return err
		}
	}

	frontend, err := fs.Sub(app.FrontendFS, "frontend")
	if err != nil {
		return err
	}
	srv := httpserver.New(st, frontend)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutdown signal received", "delay", cfg.ShutdownDelay.String())
	srv.SetNotReady()
	time.Sleep(cfg.ShutdownDelay)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	slog.Info("shutdown complete")
	return nil
}
