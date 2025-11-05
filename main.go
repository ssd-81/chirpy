package main

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	serverSecret   string
}

type formattedChirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type User struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}

type authParameters struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// ExpiresIn time.Duration `json:"expires_in_seconds"`
}

func main() {
	// for handling environment variables
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	serverSecret := os.Getenv("SERVER_SECRET")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Print("database connection failed")
		log.Fatal(err)
	}
	dbQuer := database.New(db)

	apiCfg := apiConfig{}
	apiCfg.dbQueries = dbQuer
	apiCfg.platform = platform
	apiCfg.serverSecret = serverSecret
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
	serveMux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	serveMux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)
	// webhooks handler
	serveMux.HandleFunc("POST /api/polka/webhooks", apiCfg.handlerPolkaWebhooks)

	serveMux.HandleFunc("PUT /api/users", apiCfg.handlerUpdateUser)
	serveMux.HandleFunc("GET /api/chirps", apiCfg.handlerChirpGet)
	serveMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerChirpGetSingle)
	serveMux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handlerDeleteChirp)

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

// func (cfg *apiConfig) middlewareVerifyJWT(httpHandler http.Handler) http.Handler {

// }

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
		ID:          user.ID,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed.Bool,
	}
	if err != nil {
		respondWithError(w, 404, "user could not be created")
	}
	log.Print("user created successfully")
	respondWithJSON(w, 201, userJson)

}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {

	type responseJson struct {
		Id           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		RefreshToken string    `json:"refresh_token"`
		Token        string    `json:"token"`
		IsChirpyRed  bool      `json:"is_chirpy_red"` // exp
	}

	var logParams authParameters
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&logParams)
	if err != nil {
		log.Print("error while decoding json")
		respondWithError(w, 404, "internal error occured")
	}
	log.Printf("input payload :%v", logParams)
	// keep an eye on this part
	// the logParams ExpiresIn param is optional; if the requester sets it and it is less than 1 hr
	// it will be the same; else, in other cases it will be defaulted to 1 hour
	// not required any more
	// if logParams.ExpiresIn == 0 || logParams.ExpiresIn > (60*time.Minute) {
	// 	logParams.ExpiresIn = 60 * time.Minute
	// }

	user, err := cfg.dbQueries.GetSpecificUser(r.Context(), logParams.Email)
	if err != nil {
		respondWithError(w, 404, "user not found")
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
	// the above approach does not work because of something called salt in cryptography
	// salt provides randomness in a way
	// do further reading to really understand the idea

	err = auth.CheckPasswordHash(logParams.Password, user.HashedPassword)
	if err != nil {
		respondWithError(w, 401, "401 unauthorized")
		return
	}

	// the time is fixed for the expiration of the access token
	token, err := auth.MakeJWT(user.ID, cfg.serverSecret, 60*time.Minute)
	if err != nil {
		log.Fatal("error while creation of token")
	}

	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		respondWithError(w, 500, "internal server error")
		return
	}

	// creating the refresh token entry for the user in the database
	// check for revokedAt; it might cause the tests to fail
	refreshTokensParams := database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    user.ID,
		RevokedAt: sql.NullTime{},
	}

	// do I need this somewhere?
	_, err = cfg.dbQueries.CreateRefreshToken(r.Context(), refreshTokensParams)
	if err != nil {
		respondWithError(w, 500, "could not create refresh token")
	}

	returnPayload := responseJson{
		Id:           user.ID,
		UpdatedAt:    user.UpdatedAt,
		CreatedAt:    user.CreatedAt,
		Email:        user.Email,
		Token:        token,
		RefreshToken: refreshToken,
		IsChirpyRed:  user.IsChirpyRed.Bool,
	}

	respondWithJSON(w, 200, returnPayload)

}

func (cfg *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
		// UserId uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "something went wrong")
		return
	}
	jwtToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "could not get Refresh token")
		return
	}
	// userId, err := cfg.dbQueries.GetUserFromRefreshToken(r.Context(), refreshToken)  // experimental
	// userId, err := auth.ValidateJWT(token, cfg.serverSecret)
	userId, err := auth.ValidateJWT(jwtToken, cfg.serverSecret)
	log.Println("267: ", userId) // for testing purpose
	if err != nil {
		respondWithError(w, 401, "invalid jwt")
		return
	}
	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	} else {

		stringSlice := strings.Split(params.Body, " ")
		profaneWords := []string{"kerfuffle", "sharbert", "fornax"}

		for i, val := range stringSlice {
			if slices.Contains(profaneWords, strings.ToLower(val)) {
				stringSlice[i] = "****"
			}
		}
		nonProfane := strings.Join(stringSlice, " ")

		// chirpParams := database.CreateChirpParams{
		// 	Body:   nonProfane,
		// 	UserID: uuid.NullUUID{UUID: params.UserId, Valid: true},
		// }
		chirpParams := database.CreateChirpParams{
			Body:   nonProfane,
			UserID: userId,
		}
		chirp, err := cfg.dbQueries.CreateChirp(r.Context(), chirpParams)

		if err != nil {
			respondWithError(w, 500, "chirp could not be created. try again.")
			return
		}
		strChirp := formattedChirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
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

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Token string `json:"token"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "could not get Bearer token")
		return
	}
	refreshToken, err := cfg.dbQueries.GetRefreshToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 401, "no refresh token found")
			return
		} else {
			respondWithError(w, 400, "invalid refresh token")
			return
		}
	}
	// make sure this works as expected
	if refreshToken.ExpiresAt.Time.Before(time.Now()) {
		respondWithError(w, 401, "refresh token expired")
		return
	}

	// if refreshToken.RevokedAt.Time.Before(time.Now()) {
	// respondWithError(w, 401, "refresh token revoked")
	// return
	// }

	if refreshToken.RevokedAt.Valid {
		respondWithError(w, 401, "refresh token revoked")
		return
	}

	// creating access token for the user
	// make sure the idea is solid; not sure if i am thinking right.
	// double check this section
	userID, err := cfg.dbQueries.GetUserFromRefreshToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 401, "no user found with the given refresh token")
			return
		} else {
			respondWithError(w, 400, "invalid token")
			return
		}
	}

	accessToken, err := auth.MakeJWT(userID, cfg.serverSecret, 60*time.Minute)
	if err != nil {
		respondWithError(w, 500, "failed to generate access token")
	}
	sendPayload := parameters{
		Token: accessToken,
	}
	respondWithJSON(w, 200, sendPayload)

}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "could not get Bearer token")
		return
	}
	err = cfg.dbQueries.RevokeRefreshToken(r.Context(), token)
	if err != nil {
		respondWithError(w, 500, "failed to revoke the refresh token")
		return
	}

	// no payload
	w.WriteHeader(204) // 204 ⇒ implies success; no payload
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	type responsePayload struct {
		Id          string    `json:"id"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		Email       string    `json:"email"`
		IsChirpyRed bool      `json:"is_chirpy_red"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 401, "something went wrong")
		return
	}
	if params.Email == "" || params.Password == "" {
		respondWithError(w, 400, "email and password are required")
		return
	}
	jwtToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "could not get access token")
		return
	}

	userId, err := auth.ValidateJWT(jwtToken, cfg.serverSecret)
	if err != nil {
		respondWithError(w, 401, "invalid jwt")
		return
	}
	// hashing the password
	hashedPass, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, 500, "error while hashing the password")
		return
	}
	dbParams := database.UpdateEmailAndPasswordParams{
		Email:          params.Email,
		HashedPassword: hashedPass,
		ID:             userId,
	}
	err = cfg.dbQueries.UpdateEmailAndPassword(r.Context(), dbParams)
	if err != nil {
		respondWithError(w, 500, "could not update email and password")
		return
	}
	updatedUser, err := cfg.dbQueries.GetSpecificUserWithoutPassword(r.Context(), userId)
	rp := responsePayload{
		Id:          updatedUser.Email,
		CreatedAt:   updatedUser.UpdatedAt,
		UpdatedAt:   updatedUser.UpdatedAt,
		Email:       updatedUser.Email,
		IsChirpyRed: updatedUser.IsChirpyRed.Bool,
	}
	if err != nil {
		respondWithError(w, 500, "could not retrieve updated user")
	}
	log.Print(rp)
	respondWithJSON(w, 200, rp)

}

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	jwtToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "could not get access token")
		return
	}

	userId, err := auth.ValidateJWT(jwtToken, cfg.serverSecret)
	if err != nil {
		respondWithError(w, 401, "invalid jwt")
		return
	}

	Cid := r.PathValue("chirpID")
	formattedCid := uuid.MustParse(Cid)

	author, err := cfg.dbQueries.GetChirpAuthor(r.Context(), formattedCid)
	if err != nil {
		respondWithError(w, 404, "no such chirp exists")
		return
	}
	if author != userId {
		respondWithError(w, 403, "this chirp does not belong to you")
		return
	}
	err = cfg.dbQueries.DeleteChirp(r.Context(), formattedCid)
	if err != nil {
		respondWithError(w, 500, "internal server error while deleting the chirp")
		return
	}
	w.WriteHeader(204)

}

func (cfg *apiConfig) handlerPolkaWebhooks(w http.ResponseWriter, r *http.Request) {
	
	type dataStruct struct {
		UserID string `json:"user_id"`
	}
	type requestPayload struct {
		Event string     `json:"event"`
		Data  dataStruct `json:"data"`
	}

	decoder := json.NewDecoder(r.Body)
	params := requestPayload{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "bad request")
		return
	}

	// we only care about handling the user.upgraded event
	if params.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}
	userId, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		respondWithError(w, 400, "bad request; invalid UID")
		return
	}

	err = cfg.dbQueries.UpgradeUserToChirpyRed(r.Context(), userId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 404, "no user found")
			return
		} else {
			// i am quite sure that this can be improved; this is pretty vague if you think
			respondWithError(w, 400, "bad request")
			return
		}
	}
	w.WriteHeader(204)
	// empty payload

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
