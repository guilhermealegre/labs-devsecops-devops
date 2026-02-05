// =============================================================================
// App A Backend - Go REST API with Prometheus Metrics
// =============================================================================
// Exposes:
//   - GET  /health      - Health check
//   - GET  /users       - List all users
//   - POST /users       - Create a user
//   - GET  /metrics     - Prometheus metrics
// =============================================================================

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

// Prometheus metrics
var (
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint", "status"},
	)

	dbQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Duration of database queries in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	usersTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "app_a_users_total",
			Help: "Total number of users in the database",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(dbQueryDuration)
	prometheus.MustRegister(usersTotal)
}

// User represents a user in the system
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// Database connection
var db *sql.DB

func main() {
	// Get database connection string from environment
	dbHost := getEnv("DB_HOST", "postgres")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "app_a")

	connStr := "host=" + dbHost + " port=" + dbPort + " user=" + dbUser +
		" password=" + dbPassword + " dbname=" + dbName + " sslmode=disable"

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Wait for database to be ready
	for i := 0; i < 30; i++ {
		err = db.Ping()
		if err == nil {
			break
		}
		log.Printf("Waiting for database... (%d/30)", i+1)
		time.Sleep(time.Second)
	}

	if err != nil {
		log.Fatalf("Database not available after 30 seconds: %v", err)
	}

	// Initialize database schema
	initDB()

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/users", instrumentedHandler(usersHandler))
	mux.Handle("/metrics", promhttp.Handler())

	// CORS middleware
	handler := corsMiddleware(mux)

	port := getEnv("PORT", "8080")
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func initDB() {
	start := time.Now()
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	dbQueryDuration.WithLabelValues("create_table").Observe(time.Since(start).Seconds())

	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized successfully")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func instrumentedHandler(handler func(http.ResponseWriter, *http.Request) int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := handler(w, r)
		duration := time.Since(start).Seconds()

		httpRequestDuration.WithLabelValues(
			r.Method,
			r.URL.Path,
			http.StatusText(status),
		).Observe(duration)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func usersHandler(w http.ResponseWriter, r *http.Request) int {
	switch r.Method {
	case "GET":
		return listUsers(w, r)
	case "POST":
		return createUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return http.StatusMethodNotAllowed
	}
}

func listUsers(w http.ResponseWriter, r *http.Request) int {
	start := time.Now()
	rows, err := db.Query("SELECT id, name, email, created_at FROM users ORDER BY created_at DESC")
	dbQueryDuration.WithLabelValues("list_users").Observe(time.Since(start).Seconds())

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return http.StatusInternalServerError
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	// Update users count metric
	usersTotal.Set(float64(len(users)))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
	return http.StatusOK
}

func createUser(w http.ResponseWriter, r *http.Request) int {
	var user struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return http.StatusBadRequest
	}

	if user.Name == "" || user.Email == "" {
		http.Error(w, "Name and email are required", http.StatusBadRequest)
		return http.StatusBadRequest
	}

	start := time.Now()
	var newUser User
	err := db.QueryRow(
		"INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id, name, email, created_at",
		user.Name, user.Email,
	).Scan(&newUser.ID, &newUser.Name, &newUser.Email, &newUser.CreatedAt)
	dbQueryDuration.WithLabelValues("create_user").Observe(time.Since(start).Seconds())

	if err != nil {
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newUser)
	return http.StatusCreated
}
