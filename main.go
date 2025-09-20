package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func main() {
	apiCfg := apiConfig{}
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

func handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}
	type errorJson struct {
		Error string `json:"error"`
    }
	type validJson struct {
		Valid bool `json:"valid"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		errPayload := errorJson {
			Error: "something went wrong",
		}
		dat, err := json.Marshal(errPayload)
		if err != nil {
			w.WriteHeader(500)
			log.Panicf("error while marshaling the json")
			return
		}
		w.Header().Set("Content-Type", "application/json")
    	w.WriteHeader(400)
    	w.Write(dat)
		
	}
	if len(params.Body) > 140 {
		errPayload := errorJson{
			Error: "Chirp is too long",
		}
		dat, err := json.Marshal(errPayload)
		if err != nil {
			w.WriteHeader(500)
			log.Panicf("error while marshaling the json")
			return
		}
		w.Header().Set("Content-Type", "application/json")
    	w.WriteHeader(400)
    	w.Write(dat)

	} else {
		validPayload := validJson{
			Valid: true,
		}
		dat, err := json.Marshal(validPayload)
		if err != nil {
			w.WriteHeader(500)
			log.Panicf("error while marshaling the json")
			return
		}
		w.Header().Set("Content-Type", "application/json")
    	w.WriteHeader(200)
    	w.Write(dat)

	}

}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)
	w.Write([]byte("successfully reset."))
}
