package main

import (
	"encoding/json"
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
	hitTemplate := `
	<html>
  	<body>
    	<h1>Welcome, Chirpy Admin</h1>
    	<p>Chirpy has been visited %d times!</p>
  	</body>
	</html>
	`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, hitTemplate, cfg.fileserverHits.Load())
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
	mux.HandleFunc("GET /api/healthz", healthCheck)
	mux.HandleFunc("POST /api/validate_chirp", validateChirp)
	mux.HandleFunc("GET /admin/metrics", apiCfg.fsHitsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.fsResetHandler)
	server := http.Server{Addr: ":8080", Handler: mux}
	server.ListenAndServe()
}

func healthCheck(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func validateChirp(w http.ResponseWriter, req *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}
	type returnVals struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}

	decoder := json.NewDecoder(req.Body)
	text := chirp{}
	err := decoder.Decode(&text)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response, _ := json.Marshal(returnVals{Error: fmt.Sprintf("%v", err)})
		w.Write(response)
		return
	}

	valid := len([]rune(text.Body)) <= 140
	if valid {
		w.WriteHeader(http.StatusOK)
		response, _ := json.Marshal(returnVals{Valid: len([]rune(text.Body)) <= 140})
		w.Write(response)
	} else {
		w.WriteHeader(http.StatusBadRequest)
		response, _ := json.Marshal(returnVals{Error: "chirp too long"})
		w.Write(response)
	}
}
