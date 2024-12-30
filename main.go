package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/metalmatze/signal/internalserver"
	"github.com/prometheus/client_golang/prometheus"

	_ "go.uber.org/automaxprocs"

	"github.com/kevindweb/throttle-proxy/proxyutil"
	"github.com/kevindweb/throttle-proxy/proxyutil/proxyhttp"
)

func main() {
	cfg, err := proxyutil.ParseConfigFlags()
	if err != nil {
		log.Fatalf("Failed to parse flags: %v", err)
	}

	ctx := context.Background()
	servers := make([]*http.Server, 0, 2)
	insecureServer, err := setupInsecureServer(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	if insecureServer != nil {
		servers = append(servers, insecureServer)
	}

	internalServer, err := setupInternalServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if internalServer != nil {
		servers = append(servers, internalServer)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	log.Println("\nShutting down servers...")
	for _, srv := range servers {
		if srv != nil {
			if err := srv.Shutdown(ctx); err != nil {
				log.Printf("server forced to shut down: %s\n", err)
			} else {
				log.Println("server gracefully stopped")
			}
		}
	}
}

func setupInsecureServer(ctx context.Context, cfg proxyutil.Config) (*http.Server, error) {
	if cfg.ProxyConfig.ClientTimeout == 0 {
		cfg.ProxyConfig.ClientTimeout = 2 * cfg.ReadTimeout
	}

	routes, err := proxyhttp.NewRoutes(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxymw Routes: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", routes)

	l, err := net.Listen("tcp", cfg.InsecureListenAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on insecure address: %v", err)
	}

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	go func() {
		log.Printf("Listening on %s for routes\n", l.Addr().String())
		if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
			log.Printf("Could not start server: %s\n", err)
		}
	}()

	return srv, nil
}

func setupInternalServer(cfg proxyutil.Config) (*http.Server, error) {
	if cfg.InternalListenAddress == "" {
		return nil, nil
	}

	reg, ok := prometheus.DefaultRegisterer.(*prometheus.Registry)
	if !ok {
		return nil, errors.New("failed to set up default registerer")
	}

	h := internalserver.NewHandler(
		internalserver.WithName("Internal throttle-proxy API"),
		internalserver.WithPrometheusRegistry(reg),
		internalserver.WithPProf(),
	)

	l, err := net.Listen("tcp", cfg.InternalListenAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on internal address: %v", err)
	}

	srv := &http.Server{
		Handler:      h,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	go func() {
		log.Printf("Listening on %s for metrics and pprof", l.Addr().String())
		if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
			log.Printf("Could not start server: %s\n", err)
		}
	}()

	return srv, nil
}
