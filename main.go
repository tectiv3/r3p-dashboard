package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Config: %v", err)
	}

	db, err := OpenDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("DB: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := NewBroker()

	mqttSub := NewMQTTSubscriber(cfg.MQTT, db, broker)
	if err := mqttSub.Start(); err != nil {
		log.Fatalf("MQTT: %v", err)
	}

	go runCompactor(ctx, db)

	api := NewAPI(db, broker)
	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: api.Handler(),
	}

	go func() {
		log.Printf("HTTP server listening on %s", cfg.Listen)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")

	cancel()
	mqttSub.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

func runCompactor(ctx context.Context, db *DB) {
	if err := db.Compact(); err != nil {
		log.Printf("Compaction error: %v", err)
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := db.Compact(); err != nil {
				log.Printf("Compaction error: %v", err)
			}
		}
	}
}
