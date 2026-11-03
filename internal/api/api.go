package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/FelpsSds/React-Server/internal/store/pgstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type apiHandler struct {
	q           *pgstore.Queries
	r           *chi.Mux
	upgrader    websocket.Upgrader
	subscribers map[string]map[*websocket.Conn]context.CancelFunc
	mu          *sync.Mutex
}

func (h apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.r.ServeHTTP(w, r)
}

func NewHandler(q *pgstore.Queries) http.Handler {
	a := apiHandler{
		q: q,
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
			return true
		}},
		subscribers: make(map[string]map[*websocket.Conn]context.CancelFunc),
		mu:          &sync.Mutex{},
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/subscribe/{room_id}", a.handleSubscribe)

	r.Route("/api", func(r chi.Router) {
		r.Route("/rooms", func(r chi.Router) {
			r.Post("/", a.handleCreateRoom)

			r.Route("/{room_id}/messages", func(r chi.Router) {
				r.Get("/", a.handleGetMessages)
				r.Post("/", a.handleCreateMessage)

				r.Route("/{message_id}", func(r chi.Router) {
					r.Get("/", a.handleGetRoomMessage)
					r.Patch("/react", a.handleReactToMessage)
					r.Delete("/react", a.handleRemoveReactFromMessage)
					r.Patch("/answer", a.handleMarkMessageAsAnswer)
				})
			})
		})
	})
	a.r = r
	return a
}

type Message struct {
	Kind   string `json:"kind"`
	Value  any    `json:"value"`
	RoomID string `json:"-"`
}

func (h apiHandler) notifyClients(msg Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	subscribers, ok := h.subscribers[msg.RoomID]
	if !ok || len(subscribers) == 0 {
		return
	}

	for conn, cancel := range subscribers {
		if err := conn.WriteJSON(msg); err != nil {
			slog.Warn("Failed to send message to client", "error", err)
			cancel()
		}
	}
}

func (h apiHandler) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	type _body struct {
		Theme string `json:"theme"`
	}
	var body _body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid json", http.StatusBadRequest)
		return
	}

	roomID, err := h.q.CreateRoom(r.Context())
	if err != nil {
		slog.Error("Failed to insert room", "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	type _response struct {
		ID string `json:"id"`
	}

	data, _ := json.Marshal(_response{ID: roomID[0].ID.String()})
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
func (h apiHandler) handleGetRooms(w http.ResponseWriter, r *http.Request) {
}
func (h apiHandler) handleCreateRoomMessage(w http.ResponseWriter, r *http.Request) {
	rawRoomID := chi.URLParam(r, "room_id")
		roomID, err := uuid.Parse(rawRoomID)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	_, err = h.q.GetRoom(r.Context(), roomID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Room not found", http.StatusBadRequest)
			return
		}

		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	c, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("Failed to upgrade connection", "error", err)
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}

	defer c.Close()

	ctx, cancel := context.WithCancel(r.Context())
	h.mu.Lock()
	if _, ok := h.subscribers[rawRoomID]; !ok {
		h.subscribers[rawRoomID] = make(map[*websocket.Conn]context.CancelFunc)
	}
	if h.subscribers[rawRoomID] == nil {
		h.subscribers[rawRoomID] = make(map[*websocket.Conn]context.CancelFunc)
	} else {
		h.subscribers[rawRoomID] = make(map[*websocket.Conn]context.CancelFunc)
		h.subscribers[rawRoomID][c] = cancel
	}
	slog.Info("New client connected", "room_id", rawRoomID, "client_ip", r.RemoteAddr)
	h.subscribers[rawRoomID][c] = cancel
	h.mu.Unlock()

	<-ctx.Done()

	h.mu.Lock()
	delete(h.subscribers[rawRoomID], c)

	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.subscribers[rawRoomID], c)
		if len(h.subscribers[rawRoomID]) == 0 {
			delete(h.subscribers, rawRoomID)
		}
		h.mu.Unlock()
		cancel()
	}()

	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}
func (h apiHandler) handleGetRoomMessage(w http.ResponseWriter, r *http.Request) {
}
func (h apiHandler) handleGetMessages(w http.ResponseWriter, r *http.Request) {
}
func (h apiHandler) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
}
func (h apiHandler) handleReactToMessage(w http.ResponseWriter, r *http.Request) {
}
func (h apiHandler) handleRemoveReactFromMessage(w http.ResponseWriter, r *http.Request) {
}
func (h apiHandler) handleMarkMessageAsAnswer(w http.ResponseWriter, r *http.Request) {
}
func (h apiHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	rawRoomID := chi.URLParam(r, "room_id")
	roomID, err := uuid.Parse(rawRoomID)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	_, err = h.q.GetRoom(r.Context(), roomID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Room not found", http.StatusBadRequest)
			return
		}

		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	c, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("Failed to upgrade connection", "error", err)
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}

	defer c.Close()

	ctx, cancel := context.WithCancel(r.Context())
	h.mu.Lock()
	if _, ok := h.subscribers[rawRoomID]; !ok {
		h.subscribers[rawRoomID] = make(map[*websocket.Conn]context.CancelFunc)
	}
	if h.subscribers[rawRoomID] == nil {
		h.subscribers[rawRoomID] = make(map[*websocket.Conn]context.CancelFunc)
	} else {
		h.subscribers[rawRoomID] = make(map[*websocket.Conn]context.CancelFunc)
		h.subscribers[rawRoomID][c] = cancel
	}
	slog.Info("New client connected", "room_id", rawRoomID, "client_ip", r.RemoteAddr)
	h.subscribers[rawRoomID][c] = cancel
	h.mu.Unlock()

	<-ctx.Done()

	h.mu.Lock()
	delete(h.subscribers[rawRoomID], c)

	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.subscribers[rawRoomID], c)
		if len(h.subscribers[rawRoomID]) == 0 {
			delete(h.subscribers, rawRoomID)
		}
		h.mu.Unlock()
		cancel()
	}()

	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}
