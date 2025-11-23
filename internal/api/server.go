package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/tae2089/certificate-operator/internal/api/router"
	"golang.org/x/sync/errgroup"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	apiLog = ctrl.Log.WithName("api-server")
)

// StartAPIServer starts the Gin API server using errgroup for proper error handling
func StartAPIServer(ctx context.Context, k8sClient client.Client, port string) error {
	r := router.SetupRouter(k8sClient)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	g, gCtx := errgroup.WithContext(ctx)

	// Start HTTP server in errgroup
	g.Go(func() error {
		apiLog.Info("Starting API server", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			apiLog.Error(err, "API server error")
			return err
		}
		return nil
	})

	// Handle graceful shutdown
	g.Go(func() error {
		<-gCtx.Done()
		apiLog.Info("Shutting down API server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			apiLog.Error(err, "API server shutdown error")
			return err
		}
		apiLog.Info("API server stopped gracefully")
		return nil
	})

	// Wait for all goroutines to complete and return any error
	return g.Wait()
}
