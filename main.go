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
	"github.com/ssd-81/chirpy/internal/auth"
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

type authParameters struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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
	serveMux.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	serveMux.HandleFunc("GET /api/chirps", apiCfg.handlerChirpGet)
	serveMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerChirpGetSingle)

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

	decoder := json.NewDecoder(r.Body)
	params := authParameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "something went wrong")
	}
	// create an struct for passing to CreateUser
	hashedPass, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Panic("error while hashing the password")
	}
	userParams := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPass,
	}
	// w.Write([]byte("HTTP 201 Created"))
	user, err := cfg.dbQueries.CreateUser(r.Context(), userParams)

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

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {

	type responseJson struct {
		Id        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}

	var logParams authParameters
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&logParams)
	if err != nil {
		log.Print("error while decoding json")
		respondWithError(w, 404, "internal error occured")
	}
	log.Printf("input payload :%v", logParams)

	user, err := cfg.dbQueries.GetSpecificUser(r.Context(), logParams.Email)
	if err != nil {
		respondWithError(w, 404, "not found")
		return
	}
	// hashPass, _ := auth.HashPassword(logParams.Password)
	// this is likely a mistake (we will look into the matter)
	// if user.HashedPassword != hashPass {
	// 	log.Printf("password: %v", logParams.Password)
	// 	log.Printf("user hash:%v hashpass :%v ", user.HashedPassword, hashPass)
	// 	respondWithError(w, 401, "401 unauthorized")
	// 	return
	// }

	err = auth.CheckPasswordHash(logParams.Password, user.HashedPassword)
	if err != nil {
		respondWithError(w, 401, "401 unauthorized")
		return
	}

	returnPayload := responseJson{
		Id:        user.ID,
		UpdatedAt: user.UpdatedAt,
		CreatedAt: user.CreatedAt,
		Email:     user.Email,
	}

	respondWithJSON(w, 200, returnPayload)

}

func (cfg *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body   string    `json:"body"`
		UserId uuid.UUID `json:"user_id"`
	}

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

func (cfg *apiConfig) handlerChirpGetSingle(w http.ResponseWriter, r *http.Request) {
	// retrieving specific user in the requested GET endpoint
	Cid := r.PathValue("chirpID")
	formattedCid := uuid.MustParse(Cid) // converts string to UUID
	chirp, err := cfg.dbQueries.GetChirp(r.Context(), formattedCid)
	if err != nil {
		log.Printf("error encountered while searching for chirp (GET)")
		respondWithError(w, 404, "user_id does not exist in db")
		return
	}
	fChirp := formattedChirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	respondWithJSON(w, 200, fChirp)
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
