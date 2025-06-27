package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/mrasore98/go_http_servers/internal/database"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

type returnErr struct {
	Error string `json:"error"`
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
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		panic("Error opening database")
	}
	dbQueries := database.New(db)
	apiCfg := apiConfig{fileserverHits: atomic.Int32{}, dbQueries: dbQueries}
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

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, returnErr{Error: msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	w.WriteHeader(code)
	response, _ := json.Marshal(payload)
	w.Write(response)
}

func validateChirp(w http.ResponseWriter, req *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}
	type returnVals struct {
		Valid   bool   `json:"valid,omitempty"`
		Cleaned string `json:"cleaned_body,omitempty"`
	}

	decoder := json.NewDecoder(req.Body)
	text := chirp{}
	err := decoder.Decode(&text)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, fmt.Sprintf("%v", err))
		return
	}

	valid := len([]rune(text.Body)) <= 140
	if valid {
		respondWithJSON(w, http.StatusOK, returnVals{Cleaned: removeProfanity(text.Body)})
	} else {
		respondWithError(w, http.StatusBadRequest, "chirp too long")
	}
}

func removeProfanity(msg string) string {
	profaneWords := map[string]bool{
		"kerfuffle": true,
		"sharbert":  true,
		"fornax":    true,
	}

	words := strings.Split(msg, " ")
	for idx, word := range words {
		if profaneWords[strings.ToLower(word)] {
			words[idx] = "****"
		}
	}
	return strings.Join(words, " ")
}
