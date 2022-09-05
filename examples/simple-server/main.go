package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	router := mux.NewRouter()

	router.HandleFunc("/", mainHandler)
	router.HandleFunc("/hello/{person}", helloHandler)

	fmt.Println("Listening on localhost:8080...")
	log.Fatal(http.ListenAndServe(":8080", router))
}

func mainHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("I'm here!\n"))
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	person := vars["person"]
	_, _ = w.Write([]byte(fmt.Sprintf("Hello %s!\n", person)))
}
