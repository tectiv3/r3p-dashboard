package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"
)

//go:embed web
var webFS embed.FS

type API struct {
	db     *DB
	broker *Broker
}

func NewAPI(db *DB, broker *Broker) *API {
	return &API{db: db, broker: broker}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/current", a.handleCurrent)
	mux.HandleFunc("GET /api/history", a.handleHistory)
	mux.HandleFunc("GET /api/topics", a.handleTopics)
	mux.HandleFunc("GET /api/events", a.handleEvents)
	mux.HandleFunc("GET /api/stream", a.handleStream)

	webSub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServer(http.FS(webSub)))

	return mux
}

func (a *API) handleCurrent(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryLatest()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, rows)
}

func (a *API) handleHistory(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	if topic == "" {
		http.Error(w, "topic required", 400)
		return
	}
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if to == 0 {
		to = time.Now().Unix()
	}
	if from == 0 {
		from = to - 6*3600
	}
	rows, err := a.db.QueryHistory(topic, from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, rows)
}

func (a *API) handleTopics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, TopicRegistry)
}

func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if to == 0 {
		to = time.Now().Unix()
	}
	if from == 0 {
		from = to - 86400
	}
	rows, err := a.db.QueryEvents(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, rows)
}

func (a *API) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := a.broker.Subscribe()
	defer a.broker.Unsubscribe(ch)

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case update := <-ch:
			data, _ := json.Marshal(update)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ":keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("JSON encode error: %v", err)
	}
}
