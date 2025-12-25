package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi"
	"golang.org/x/crypto/bcrypt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	pool, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_URL"))
	if err != nil {
		logger.Error("unable to connect to PostgreSQL",
			slog.String("error", err.Error()),
		)
		panic(err)
	}

	err = pool.Ping(context.Background())
	if err != nil {
		logger.Error("failed to connect to PostgreSQL",
			slog.String("error", err.Error()),
		)
		pool.Close()
		panic(err)
	}
	defer pool.Close()

	server := &http.Server{
		Addr:    ":4000",
		Handler: Routes(logger, pool),
	}

	logger.Info("starting server", "addr", ":4000")
	err = server.ListenAndServe()
	if err != nil {
		logger.Error(err.Error())
		panic(err)
	}
}

func Routes(logger *slog.Logger, dbPool *pgxpool.Pool) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{"message": "Ok"}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("failed to encode a JSON response", slog.String("error", err.Error()))
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	})

	userHandler := UserHandler{UserHandlerOptions{logger: logger, dbPool: dbPool}}
	r.Route("/users", func(r chi.Router) {
		r.Get("/", userHandler.FindAll)
		r.Get("/{id}", userHandler.FindOne)
		r.Post("/", userHandler.Create)
		r.Delete("/{id}", userHandler.Delete)
	})

	return r
}

type CreateUserReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserModel struct {
	Id        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"password,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type UserHandler struct {
	UserHandlerOptions
}

type UserHandlerOptions struct {
	logger *slog.Logger
	dbPool *pgxpool.Pool
}

func (u UserHandler) FindAll(w http.ResponseWriter, r *http.Request) {
	// Find all users from the DB
	query := "SELECT id, name, email, created_at FROM users"
	rows, err := u.dbPool.Query(r.Context(), query)
	if err != nil {
		u.logger.Error("failed to execute find all users query", slog.String("error", err.Error()))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	// Collect all relevant rows and convert them into Go Struct
	users, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[UserModel])
	if err != nil {
		u.logger.Error("failed to collect rows for find all users query", slog.String("error", err.Error()))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Return a response body
	response := map[string][]UserModel{"data": users}
	buffer := new(bytes.Buffer)
	err = json.NewEncoder(buffer).Encode(response)
	if err != nil {
		u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "applications/json")
	w.Write(buffer.Bytes())
}

func (u UserHandler) FindOne(w http.ResponseWriter, r *http.Request) {
	// Convert from string userId to int
	userId, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		u.logger.Error("failed to convert a string user id to int", slog.String("error", err.Error()))

		response := map[string]string{"message": "User ID must be a valid value"}
		buffer := new(bytes.Buffer)
		err = json.NewEncoder(buffer).Encode(response)
		if err != nil {
			u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(buffer.Bytes())
		return
	}

	// Find the user in the DB
	query := "SELECT id, name, email, created_at FROM users WHERE id=$1"
	rows, err := u.dbPool.Query(r.Context(), query, userId)
	if err != nil {
		u.logger.Error("failed to execute a find one query", slog.String("error", err.Error()))

		response := map[string]string{"message": "Internal Server Error"}
		buffer := new(bytes.Buffer)
		err = json.NewEncoder(buffer).Encode(response)
		if err != nil {
			u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(buffer.Bytes())
		return
	}

	defer rows.Close()

	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByNameLax[UserModel])
	if err != nil {
		u.logger.Error("failed to find a user", slog.String("error", err.Error()))

		if errors.Is(err, pgx.ErrNoRows) {
			response := map[string]string{"message": "User does not exist"}
			buffer := new(bytes.Buffer)
			err = json.NewEncoder(buffer).Encode(response)
			if err != nil {
				u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "applications/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write(buffer.Bytes())
			return
		}

		response := map[string]string{"message": "Internal Server Error"}
		buffer := new(bytes.Buffer)
		err = json.NewEncoder(buffer).Encode(response)
		if err != nil {
			u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(buffer.Bytes())
		return
	}

	// Return a response body
	buffer := new(bytes.Buffer)
	err = json.NewEncoder(buffer).Encode(user)
	if err != nil {
		u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "applications/json")
	w.Write(buffer.Bytes())
}

func (u UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Transform JSON to Struct
	var createUserReq CreateUserReq
	if err := json.NewDecoder(r.Body).Decode(&createUserReq); err != nil {
		u.logger.Error("failed to decode create user request", slog.String("error", err.Error()))

		response := map[string]string{"message": "Request body is not a valid JSON."}
		buffer := new(bytes.Buffer)
		err = json.NewEncoder(buffer).Encode(response)
		if err != nil {
			u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(buffer.Bytes())
		return
	}

	// Hash password field
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(createUserReq.Password), 12)
	if err != nil {
		u.logger.Error("failed to hash a password", slog.String("error", err.Error()))

		response := map[string]string{"message": "Internal Server Error"}
		buffer := new(bytes.Buffer)
		err = json.NewEncoder(buffer).Encode(response)
		if err != nil {
			u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(buffer.Bytes())
		return
	}

	// Insert a new user to DB
	query := "INSERT INTO users(name, email, password) VALUES($1, $2, $3) RETURNING id, name, email, password, created_at"

	var insertedUser UserModel
	err = u.dbPool.QueryRow(r.Context(), query, createUserReq.Name, createUserReq.Email, hashedPassword).Scan(
		&insertedUser.Id,
		&insertedUser.Name,
		&insertedUser.Email,
		&insertedUser.Password,
		&insertedUser.CreatedAt,
	)
	if err != nil {
		u.logger.Error("failed to insert a new user", slog.String("error", err.Error()))

		response := map[string]string{"message": "Internal Server Error"}
		buffer := new(bytes.Buffer)
		err = json.NewEncoder(buffer).Encode(response)
		if err != nil {
			u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(buffer.Bytes())
		return
	}

	// Return a response body
	insertedUser.Password = ""
	buffer := new(bytes.Buffer)
	err = json.NewEncoder(buffer).Encode(insertedUser)
	if err != nil {
		u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(buffer.Bytes())
}

func (u UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Convert from string userId to int
	userId, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		u.logger.Error("failed to convert a string user id to int", slog.String("error", err.Error()))

		response := map[string]string{"message": "User ID must be a valid value"}
		buffer := new(bytes.Buffer)
		err = json.NewEncoder(buffer).Encode(response)
		if err != nil {
			u.logger.Error("unexpected error during json.NewEncoder()", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "applications/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(buffer.Bytes())
		return
	}

	// Execute delete query
	query := "DELETE FROM users WHERE id = $1"
	cmdTag, err := u.dbPool.Exec(r.Context(), query, userId)
	if err != nil {
		u.logger.Error("failed to execute delete user query", slog.String("error", err.Error()))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Check if the query affects how many rows
	if cmdTag.RowsAffected() == 0 {
		u.logger.Error("user ID is not found", slog.Int("userID", userId))
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	// Return a HTTP response
	w.WriteHeader(http.StatusOK)
}
