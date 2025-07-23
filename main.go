package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/mrasore98/go_http_servers/internal/database"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

// Create a wrapper User struct to control JSON keys
type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
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
	if platform := os.Getenv("PLATFORM"); platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
	}
	cfg.fileserverHits.Store(0)
	if err := cfg.dbQueries.ClearUsers(req.Context()); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if err := cfg.dbQueries.ClearChirps(req.Context()); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}

func (cfg *apiConfig) listChirps(w http.ResponseWriter, req *http.Request) {
	chirps, err := cfg.dbQueries.GetAllChirps(req.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error listing chirps")
		return
	}
	var respChirps []Chirp
	for _, oldChirp := range chirps {
		newChirp := Chirp{ID: oldChirp.ID, CreatedAt: oldChirp.CreatedAt, UpdatedAt: oldChirp.UpdatedAt, Body: oldChirp.Body, UserID: oldChirp.UserID.UUID}
		respChirps = append(respChirps, newChirp)
	}
	respondWithJSON(w, http.StatusOK, respChirps)
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, req *http.Request) {
	type postData struct {
		Body   string        `json:"body"`
		UserID uuid.NullUUID `json:"user_id"`
	}
	params := postData{}
	if err := json.NewDecoder(req.Body).Decode(&params); err != nil {
		respondWithError(w, http.StatusBadRequest, "error reading payload data")
		return
	}
	// fmt.Printf("request: %v", req.Body)
	// fmt.Printf("chirp body: %v\nuser id: %v", params.Body, params.UserID)

	// Validate chirp contents
	validatedBody, err := validateChirp(params.Body)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, fmt.Sprintf("%v", err))
	}
	// Replace body with validated and cleaned contents before adding to the database
	newParams := database.CreateChirpParams{
		Body:   validatedBody,
		UserID: params.UserID,
	}

	chirp, err := cfg.dbQueries.CreateChirp(req.Context(), newParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not create chirp")
		return
	}

	returnChirp := Chirp{
		chirp.ID,
		chirp.CreatedAt,
		chirp.UpdatedAt,
		chirp.Body,
		chirp.UserID.UUID,
	}
	respondWithJSON(w, http.StatusCreated, returnChirp)
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, req *http.Request) {
	type postData struct {
		Email string `json:"email"`
	}

	params := postData{}
	if err := json.NewDecoder(req.Body).Decode(&params); err != nil {
		respondWithError(w, http.StatusBadRequest, "error reading payload data")
		return
	}

	user, err := cfg.dbQueries.CreateUser(req.Context(), params.Email)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not create user")
	}

	newUser := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.CreatedAt,
		Email:     user.Email,
	}

	respondWithJSON(w, http.StatusCreated, newUser)
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
	// "App" endpoints - serves static files
	fsHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fsHandler))
	// API endpoints
	mux.HandleFunc("GET /api/healthz", healthCheck)
	mux.HandleFunc("POST /api/chirps", apiCfg.createChirp)
	mux.HandleFunc("GET /api/chirps", apiCfg.listChirps)
	mux.HandleFunc("POST /api/users", apiCfg.createUser)
	// Admin endpoints
	mux.HandleFunc("GET /admin/metrics", apiCfg.fsHitsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.fsResetHandler)
	// Start server
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	response, _ := json.Marshal(payload)
	w.Write(response)
}

func validateChirp(content string) (string, error) {
	if valid := len([]rune(content)) <= 140; !valid {
		return "", fmt.Errorf("chirp too long")
	}
	return removeProfanity(content), nil
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
