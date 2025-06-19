package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) fsHitsHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "Hits: %v", cfg.fileserverHits.Load())
}

func (cfg *apiConfig) fsResetHandler(w http.ResponseWriter, req *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
}

func main() {
	apiCfg := apiConfig{fileserverHits: atomic.Int32{}}
	mux := http.NewServeMux()
	fsHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fsHandler))
	mux.HandleFunc("GET /healthz", healthCheck)
	mux.HandleFunc("GET /metrics", apiCfg.fsHitsHandler)
	mux.HandleFunc("POST /reset", apiCfg.fsResetHandler)
	server := http.Server{Addr: ":8080", Handler: mux}
	server.ListenAndServe()
}

func healthCheck(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
