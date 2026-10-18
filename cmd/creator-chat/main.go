package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"example.com/creator-chat-rooms/chatroom"
	"example.com/creator-chat-rooms/infrai"
)

type server struct {
	rooms *chatroom.Service
}

func main() {
	client, err := infrai.NewClient(os.Getenv("INFRAI_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	s := &server{rooms: chatroom.NewService(client)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rooms/open", s.openRoom)
	mux.HandleFunc("POST /assets/update", s.publishUpdate)
	mux.HandleFunc("GET /rooms/{channel}/presence", s.presence)
	mux.HandleFunc("DELETE /rooms/{channel}", s.closeRoom)

	address := os.Getenv("ADDR")
	if address == "" {
		address = ":8080"
	}
	log.Printf("creator chat listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func (s *server) openRoom(w http.ResponseWriter, r *http.Request) {
	var input chatroom.OpenRoomInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.rooms.OpenRoom(r.Context(), input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) publishUpdate(w http.ResponseWriter, r *http.Request) {
	var update chatroom.ProcessingUpdate
	if err := decodeJSON(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	event, err := s.rooms.PublishUpdate(r.Context(), update)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"event": event})
}

func (s *server) presence(w http.ResponseWriter, r *http.Request) {
	result, err := s.rooms.Presence(r.Context(), r.PathValue("channel"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) closeRoom(w http.ResponseWriter, r *http.Request) {
	if err := s.rooms.CloseRoom(r.Context(), r.PathValue("channel")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("request body must be valid JSON with known fields")
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	var apiErr *infrai.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status < 400 || status > 499 {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]any{"error": map[string]string{"code": apiErr.Code, "message": apiErr.Message}})
		return
	}
	if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unsupported content state") {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeError(w, http.StatusBadGateway, errors.New("realtime request could not be completed"))
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
