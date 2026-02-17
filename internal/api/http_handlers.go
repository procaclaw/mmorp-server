package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	authapp "mmorp-server/internal/app/auth"
	charapp "mmorp-server/internal/app/character"
	worldapp "mmorp-server/internal/app/world"
)

type Handler struct {
	logger      zerolog.Logger
	auth        *authapp.Service
	characters  *charapp.Service
	world       *worldapp.Service
	corsOrigin  string
	maxBodySize int64
}

type contextKey string

const userIDContextKey contextKey = "user_id"

func NewHandler(logger zerolog.Logger, auth *authapp.Service, characters *charapp.Service, world *worldapp.Service, corsOrigin string, maxBodySize int64) *Handler {
	return &Handler{logger: logger, auth: auth, characters: characters, world: world, corsOrigin: corsOrigin, maxBodySize: maxBodySize}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(20 * time.Second))
	r.Use(h.cors)

	r.Get("/healthz", h.health)
	r.Get("/readyz", h.ready)

	r.Route("/v1", func(v1 chi.Router) {
		v1.Post("/auth/register", h.register)
		v1.Post("/auth/login", h.login)
		v1.Get("/world/state", h.worldState)
		v1.Get("/world/players", h.worldPlayers)
		v1.Get("/world/ws", h.worldWS)

		v1.Group(func(protected chi.Router) {
			protected.Use(h.authMiddleware)
			protected.Get("/characters", h.listCharacters)
			protected.Post("/characters", h.createCharacter)
			protected.Get("/characters/{characterID}", h.getCharacter)
		})
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !h.decodeBody(w, r, &req) {
		return
	}
	res, err := h.auth.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, authapp.ErrEmailInUse) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !h.decodeBody(w, r, &req) {
		return
	}
	res, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) listCharacters(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	chars, err := h.characters.ListByUser(r.Context(), uid)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", uid.String()).Msg("list characters failed")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": chars})
}

func (h *Handler) createCharacter(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var req struct {
		Name  string `json:"name"`
		Class string `json:"class"`
	}
	if !h.decodeBody(w, r, &req) {
		return
	}
	c, err := h.characters.Create(r.Context(), uid, req.Name, req.Class)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) getCharacter(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	cid, err := uuid.Parse(chi.URLParam(r, "characterID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid character id"})
		return
	}
	c, err := h.characters.GetByIDForUser(r.Context(), uid, cid)
	if err != nil {
		if errors.Is(err, charapp.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "character not found"})
			return
		}
		if errors.Is(err, charapp.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) worldState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.world.WorldState())
}

func (h *Handler) worldPlayers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"players": h.world.OnlinePlayers()})
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

func (h *Handler) worldWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing token"})
		return
	}
	uid, err := h.auth.ParseToken(token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid token"})
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Warn().Err(err).Msg("websocket upgrade failed")
		return
	}

	client := h.world.RegisterClient(conn, uid)
	go h.writePump(client)
	h.readPump(r.Context(), client)
}

func (h *Handler) readPump(ctx context.Context, client *worldapp.Client) {
	defer h.world.UnregisterClient(ctx, client)
	if client.Conn == nil {
		return
	}
	client.Conn.SetReadLimit(2048)
	_ = client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		_ = client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg struct {
			Type        string  `json:"type"`
			CharacterID string  `json:"character_id"`
			DX          float64 `json:"dx"`
			DY          float64 `json:"dy"`
			TargetID    string  `json:"target_id"`
		}
		if err := client.Conn.ReadJSON(&msg); err != nil {
			return
		}

		switch msg.Type {
		case "join":
			cid, err := uuid.Parse(msg.CharacterID)
			if err != nil {
				h.sendError(client, "invalid character_id")
				continue
			}
			char, err := h.characters.GetByIDForUser(ctx, client.AccountID, cid)
			if err != nil {
				h.sendError(client, "character not found")
				continue
			}
			h.world.Join(client, char)
		case "move":
			h.world.Move(client, msg.DX, msg.DY)
		case "attack":
			if strings.TrimSpace(msg.TargetID) == "" {
				h.sendError(client, "target_id is required")
				continue
			}
			h.world.Attack(client, msg.TargetID)
		default:
			h.sendError(client, "unknown message type")
		}
	}
}

func (h *Handler) writePump(client *worldapp.Client) {
	if client.Conn == nil {
		return
	}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-client.Send:
			if !ok {
				_ = client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Handler) sendError(client *worldapp.Client, msg string) {
	b, err := json.Marshal(map[string]any{"type": "error", "message": msg})
	if err != nil {
		return
	}
	select {
	case client.Send <- b:
	default:
	}
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing bearer token"})
			return
		}
		uid, err := h.auth.ParseToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid token"})
			return
		}
		ctx := context.WithValue(r.Context(), userIDContextKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(userIDContextKey)
	uid, ok := v.(uuid.UUID)
	return uid, ok
}

func (h *Handler) cors(next http.Handler) http.Handler {
	origin := h.corsOrigin
	if origin == "" {
		origin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
