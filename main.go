package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/ssd-81/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

func main() {
	// for handling environment variables
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Print("database connection failed")
		log.Fatal(err)
	}
	dbQuer := database.New(db)

	apiCfg := apiConfig{}
	apiCfg.dbQueries = dbQuer
	serveMux := http.NewServeMux()
	// serveMux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	serveMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	serveMux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK")) // seems unlikely
	})
	// introducing admin namespace
	serveMux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	serveMux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	serveMux.HandleFunc("POST /api/validate_chirp", handlerValidateChirp)
	serveMux.HandleFunc("POST /api/users", handlerUsers)

	server := http.Server{}
	server.Handler = serveMux
	server.Addr = ":8080"

	log.Printf("Serving files from %s on port: %s\n", "/app", "8080")
	log.Fatal(server.ListenAndServe())

}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	// cfg.fileserverHits.Store(cfg.fileserverHits.Add(1))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})

}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	responseString := fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())
	w.Write([]byte(responseString))
}

func handlerUsers(w http.ResponseWriter, r *http.Request){
	type parameters struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

}

func handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}
	type validJson struct {
		CleanedBody string `json:"cleaned_body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w , 400, "something went wrong")
	}
	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
	} else {

		stringSlice := strings.Split(params.Body, " ")
		profaneWords := []string{"kerfuffle", "sharbert", "fornax"}

		for i, val := range stringSlice {
			if slices.Contains(profaneWords, strings.ToLower(val)) {
				stringSlice[i] = "****"
			}
		}
		nonProfane := strings.Join(stringSlice, " ")

		validPayload := validJson{
			CleanedBody: nonProfane,
		}
		
		respondWithJSON(w, 200,validPayload)
	}

}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)
	w.Write([]byte("successfully reset."))
}

func respondWithError(w http.ResponseWriter, code int, msg string){
	type errorJson struct {
		Error string `json:"error"`
	}
	errPayLoad := errorJson{Error: msg}
	dat, err := json.Marshal(errPayLoad)
	if err != nil {
		w.WriteHeader(500)
		log.Panicf("error while marshaling the json")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
	
}


func respondWithJSON(w http.ResponseWriter, code int, payload interface{}){
	dat, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(500)
		log.Panicf("error while marshaling the json")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}
