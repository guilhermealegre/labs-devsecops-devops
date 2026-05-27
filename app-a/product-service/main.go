// product-service: CRUD for products, exposes /health /products /metrics
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

	productsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "product_service_products_total",
		Help: "Total number of products",
	})
)

func init() {
	prometheus.MustRegister(httpDuration, productsTotal)
}

type Product struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	CreatedAt   time.Time `json:"created_at"`
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

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS products (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		price NUMERIC(10,2) NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatalf("init schema: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "product-service"})
	})
	mux.HandleFunc("/products", cors(instrument(productsHandler)))
	mux.Handle("/metrics", promhttp.Handler())

	port := getEnv("PORT", "8081")
	log.Printf("product-service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func productsHandler(w http.ResponseWriter, r *http.Request) int {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, name, description, price, created_at FROM products ORDER BY created_at DESC")
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return http.StatusInternalServerError
		}
		defer rows.Close()
		var products []Product
		for rows.Next() {
			var p Product
			rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.CreatedAt)
			products = append(products, p)
		}
		productsTotal.Set(float64(len(products)))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(products)
		return http.StatusOK

	case http.MethodPost:
		var req struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return http.StatusBadRequest
		}
		var p Product
		err := db.QueryRow(
			"INSERT INTO products (name, description, price) VALUES ($1, $2, $3) RETURNING id, name, description, price, created_at",
			req.Name, req.Description, req.Price,
		).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.CreatedAt)
		if err != nil {
			http.Error(w, "failed to create product", http.StatusInternalServerError)
			return http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
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
