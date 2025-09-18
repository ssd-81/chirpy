package main

import (
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

	serveMux.HandleFunc("GET /app/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK")) // seems unlikely
	})
	serveMux.HandleFunc("GET /api/metrics", apiCfg.handlerMetrics)
	serveMux.HandleFunc("POST /api/reset", apiCfg.handlerReset)
	server := http.Server{}
	server.Handler = serveMux
	server.Addr = ":8080"

	log.Printf("Serving files from %s on port: %s\n", "/app", "8080")
	log.Fatal(server.ListenAndServe())

}

// func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
// 	cfg.fileserverHits.Store(cfg.fileserverHits.Add(1))
// 	return next
// }

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	// cfg.fileserverHits.Store(cfg.fileserverHits.Add(1))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})

}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	responseString := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())
	w.Write([]byte(responseString))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)
	w.Write([]byte("successfully reset."))
}
