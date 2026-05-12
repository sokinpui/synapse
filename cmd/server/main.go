package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sokinpui/synapse.go/internal/broker"
	"github.com/sokinpui/synapse.go/internal/config"
	"github.com/sokinpui/synapse.go/internal/model"
	"github.com/sokinpui/synapse.go/internal/server"
	"github.com/sokinpui/synapse.go/internal/worker"
)

func main() {
	log.SetPrefix("server: ")

	configPath := flag.String("f", "config.yaml", "path to configuration file")
	flag.Parse()

	cfg := config.Load(*configPath)

	llmRegistry, err := model.New(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize LLM registry: %v", err)
	}

	memBroker := broker.NewMemoryBroker(1000)

	concurrency := cfg.Worker.ConcurrencyMultiplier * runtime.NumCPU()
	w := worker.New(memBroker, llmRegistry, concurrency)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go w.Run(ctx)

	// HTTP Server
	mux := http.NewServeMux()
	httpSrv := server.NewHTTPServer(memBroker, llmRegistry)
	httpSrv.RegisterRoutes(mux)
	httpAddr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
	hSrv := &http.Server{Addr: httpAddr, Handler: mux}

	log.Printf("HTTP Server listening at %s", httpAddr)

	go func() {
		if err := hSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down servers...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(10*runtime.NumCPU())*time.Second)
	defer cancel()

	if err := hSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
}
