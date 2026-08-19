package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	vectorscan "github.com/badrikmodi/VectorScan"
	"github.com/badrikmodi/VectorScan/httpapi"
)

const defaultSnapshotPath = "vectorscan.snapshot.json"

func main() {
	snapshotPath := os.Getenv("VECTORSCAN_SNAPSHOT")
	if snapshotPath == "" {
		snapshotPath = defaultSnapshotPath
	}

	db, err := vectorscan.Load(snapshotPath)
	switch {
	case err == nil:
		log.Printf("loaded snapshot %s (%d vectors, dimension %d)", snapshotPath, db.Len(), db.Dimension())
	case errors.Is(err, os.ErrNotExist):
		db = vectorscan.NewDB()
		log.Printf("no snapshot found at %s; starting empty", snapshotPath)
	default:
		log.Fatalf("load snapshot: %v", err)
	}

	api := httpapi.New(db)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           api,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("VectorScan listening on %s", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	case <-sigCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}

	if err := db.Save(snapshotPath); err != nil {
		log.Fatalf("save snapshot: %v", err)
	}
	log.Printf("saved snapshot %s (%d vectors)", snapshotPath, db.Len())
}
