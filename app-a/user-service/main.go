// user-service: CRUD for users, exposes /health /users /metrics
package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "github.com/lib/pq"
)

var (
	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "endpoint", "status"})

	usersTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "user_service_users_total",
		Help: "Total number of users",
	})
)

func init() {
	prometheus.MustRegister(httpDuration, usersTotal)
}

type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

var db *sql.DB

func main() {
	connStr := "host=" + getEnv("DB_HOST", "localhost") +
		" port=" + getEnv("DB_PORT", "5432") +
		" user=" + getEnv("DB_USER", "postgres") +
		" password=" + getEnv("DB_PASSWORD", "postgres") +
		" dbname=" + getEnv("DB_NAME", "app_a") +
		" sslmode=" + getEnv("DB_SSLMODE", "disable")

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	for i := 0; i < 30; i++ {
		if db.Ping() == nil {
			break
		}
		log.Printf("waiting for db (%d/30)...", i+1)
		time.Sleep(time.Second)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatalf("init schema: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "user-service"})
	})
	mux.HandleFunc("/users", cors(instrument(usersHandler)))
	mux.Handle("/metrics", promhttp.Handler())

	port := getEnv("PORT", "8080")
	log.Printf("user-service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func usersHandler(w http.ResponseWriter, r *http.Request) int {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, name, email, created_at FROM users ORDER BY created_at DESC")
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return http.StatusInternalServerError
		}
		defer rows.Close()
		var users []User
		for rows.Next() {
			var u User
			rows.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt)
			users = append(users, u)
		}
		usersTotal.Set(float64(len(users)))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
		return http.StatusOK

	case http.MethodPost:
		var req struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Email == "" {
			http.Error(w, "name and email required", http.StatusBadRequest)
			return http.StatusBadRequest
		}
		var u User
		err := db.QueryRow(
			"INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id, name, email, created_at",
			req.Name, req.Email,
		).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt)
		if err != nil {
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(u)
		return http.StatusCreated

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return http.StatusMethodNotAllowed
	}
}

func instrument(h func(http.ResponseWriter, *http.Request) int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := h(w, r)
		httpDuration.WithLabelValues(r.Method, r.URL.Path, http.StatusText(status)).Observe(time.Since(start).Seconds())
	}
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		h(w, r)
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
