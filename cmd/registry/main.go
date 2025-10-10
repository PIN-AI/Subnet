package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"subnet/internal/logging"
	"subnet/internal/registry"
)

func main() {
	var (
		grpcAddr = flag.String("grpc", ":8091", "Registry gRPC listen address (empty to disable)")
		httpAddr = flag.String("http", ":8092", "Registry HTTP listen address (empty to disable)")
	)
	flag.Parse()

	// Setup logger
	logger := logging.NewDefaultLogger()

	logger.Info("Starting Agent Registry Service", "grpc", *grpcAddr, "http", *httpAddr)

	// Create registry service
	service := registry.NewService(*grpcAddr, *httpAddr)

	// Start service
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start registry service: %v", err)
	}
	defer service.Stop()

	logger.Info("Registry service started", "grpc", *grpcAddr, "http", *httpAddr)

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh
	fmt.Println("\nShutting down registry service...")
	service.Stop()
}
