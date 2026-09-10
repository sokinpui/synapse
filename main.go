package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

func main() {
	log.SetPrefix("server: ")

	cfg := LoadConfig()

	llmRegistry, err := NewRegistry(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize LLM registry: %v", err)
	}

	memBroker := NewMemoryBroker(1000)

	concurrency := cfg.Worker.ConcurrencyMultiplier * runtime.NumCPU()
	w := NewWorker(memBroker, llmRegistry, concurrency)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go w.Run(ctx)

	mux := http.NewServeMux()
	httpSrv := NewHTTPServer(memBroker, llmRegistry)
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
