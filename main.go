package main

import (
	"log"
	"net/http"
)

func main() {

	serveMux := http.NewServeMux()
	serveMux.Handle("/", http.FileServer(http.Dir(".")))
	serveMux.Handle("/assets/logo.png", http.FileServer(http.Dir("./assets/logo.png")))
	server := http.Server{}
	server.Handler = serveMux
	server.Addr = ":8080"
	log.Printf("Serving files from %s on port: %s\n", ".", "8080")
	log.Fatal(server.ListenAndServe())

}
