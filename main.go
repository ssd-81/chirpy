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
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/ssd-81/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
}

type formattedChirp struct {
	ID        uuid.UUID     `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Body      string        `json:"body"`
	UserID    uuid.NullUUID `json:"user_id"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func main() {
	// for handling environment variables
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Print("database connection failed")
		log.Fatal(err)
	}
	dbQuer := database.New(db)

	apiCfg := apiConfig{}
	apiCfg.dbQueries = dbQuer
	apiCfg.platform = platform
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
	// serveMux.HandleFunc("POST /api/validate_chirp", handlerValidateChirp)
	serveMux.HandleFunc("POST /api/users", apiCfg.handlerUsers)
	serveMux.HandleFunc("POST /api/chirps", apiCfg.handlerChirp)
	serveMux.HandleFunc("GET /api/chirps", apiCfg.handlerChirpGet)
	serveMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerChirpGetOne)

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

func (cfg *apiConfig) handlerUsers(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "something went wrong")
	}
	// w.Write([]byte("HTTP 201 Created"))
	user, err := cfg.dbQueries.CreateUser(r.Context(), params.Email)
	// using User struct for controlling json keys
	userJson := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}
	if err != nil {
		respondWithError(w, 404, "user could not be created")
	}
	log.Print("user created successfully")
	respondWithJSON(w, 201, userJson)

}

func (cfg *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body   string    `json:"body"`
		UserId uuid.UUID `json:"user_id"` // skeptical if this going to work as expected
	}

	// type validJson struct {
	// 	CleanedBody string `json:"cleaned_body"`
	// }

	// validating if the chirp is valid
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "something went wrong")
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

		chirpParams := database.CreateChirpParams{
			Body:   nonProfane,
			UserID: uuid.NullUUID{UUID: params.UserId, Valid: true},
		}
		chirp, err := cfg.dbQueries.CreateChirp(r.Context(), chirpParams)
		strChirp := formattedChirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}
		if err != nil {
			respondWithError(w, 404, "chirp could be created. try again.")
		}
		respondWithJSON(w, 201, strChirp)

	}
}

func (cfg *apiConfig) handlerChirpGet(w http.ResponseWriter, r *http.Request) {
	dbChirps, err := cfg.dbQueries.GetChirpsAsc(r.Context())
	if err != nil {
		respondWithError(w, 404, "error occured while retriving chirps")
	}
	var slice []formattedChirp
	for _, val := range dbChirps {
		temp := formattedChirp{
			ID:        val.ID,
			CreatedAt: val.CreatedAt,
			UpdatedAt: val.UpdatedAt,
			Body:      val.Body,
			UserID:    val.UserID,
		}
		slice = append(slice, temp)
	}
	json.NewEncoder(w).Encode(slice)

}

func (cfg *apiConfig) handlerChirpGetOne(w http.ResponseWriter, r *http.Request) {
	Cid := r.PathValue("chirpID")
	// formattedCid :=
	cfg.dbQueries.GetChirp(r.Context(), formattedCid)
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(403)
		w.Write([]byte("403 Forbidden"))
		return // is this even required
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)
	cfg.dbQueries.DeleteUsers(r.Context())
	// w.Write([]byte("successfully reset."))
	w.Write([]byte("accessing this dangerous endpoint locally"))

}

func respondWithError(w http.ResponseWriter, code int, msg string) {
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

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(500)
		log.Panicf("error while marshaling the json")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}
