package http

import (
	"encoding/json"
	"log"
	"net/http"
)

type Handler struct{}

const functionsDir = "functions"

func (h Handler) ListFunctions(w http.ResponseWriter, r *http.Request) {
	functions, err := listDirectoryItems(functionsDir)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)

		log.Println(err)

		return
	}

	bytes, err := json.Marshal(functions)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		log.Println(err)

		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}