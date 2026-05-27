// order-service: manages orders and calls user-service + product-service via HTTP
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

	ordersTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "order_service_orders_total",
		Help: "Total number of orders",
	})
)

func init() {
	prometheus.MustRegister(httpDuration, ordersTotal)
}

type Order struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	ProductID int       `json:"product_id"`
	Quantity  int       `json:"quantity"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	db             *sql.DB
	userServiceURL string
	productSvcURL  string
)

func main() {
	userServiceURL = getEnv("USER_SERVICE_URL", "http://user-service:8080")
	productSvcURL = getEnv("PRODUCT_SERVICE_URL", "http://product-service:8081")

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

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS orders (
		id SERIAL PRIMARY KEY,
		user_id INT NOT NULL,
		product_id INT NOT NULL,
		quantity INT NOT NULL DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatalf("init schema: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "order-service"})
	})
	mux.HandleFunc("/orders", cors(instrument(ordersHandler)))
	mux.Handle("/metrics", promhttp.Handler())

	port := getEnv("PORT", "8082")
	log.Printf("order-service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func ordersHandler(w http.ResponseWriter, r *http.Request) int {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, user_id, product_id, quantity, created_at FROM orders ORDER BY created_at DESC")
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return http.StatusInternalServerError
		}
		defer rows.Close()
		var orders []Order
		for rows.Next() {
			var o Order
			rows.Scan(&o.ID, &o.UserID, &o.ProductID, &o.Quantity, &o.CreatedAt)
			orders = append(orders, o)
		}
		ordersTotal.Set(float64(len(orders)))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orders)
		return http.StatusOK

	case http.MethodPost:
		var req struct {
			UserID    int `json:"user_id"`
			ProductID int `json:"product_id"`
			Quantity  int `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 || req.ProductID == 0 {
			http.Error(w, "user_id and product_id required", http.StatusBadRequest)
			return http.StatusBadRequest
		}
		if req.Quantity <= 0 {
			req.Quantity = 1
		}

		// Validate user exists
		if err := ping(fmt.Sprintf("%s/users", userServiceURL)); err != nil {
			http.Error(w, "user-service unavailable", http.StatusServiceUnavailable)
			return http.StatusServiceUnavailable
		}
		// Validate product exists
		if err := ping(fmt.Sprintf("%s/products", productSvcURL)); err != nil {
			http.Error(w, "product-service unavailable", http.StatusServiceUnavailable)
			return http.StatusServiceUnavailable
		}

		var o Order
		err := db.QueryRow(
			"INSERT INTO orders (user_id, product_id, quantity) VALUES ($1, $2, $3) RETURNING id, user_id, product_id, quantity, created_at",
			req.UserID, req.ProductID, req.Quantity,
		).Scan(&o.ID, &o.UserID, &o.ProductID, &o.Quantity, &o.CreatedAt)
		if err != nil {
			http.Error(w, "failed to create order", http.StatusInternalServerError)
			return http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(o)
		return http.StatusCreated

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return http.StatusMethodNotAllowed
	}
}

func ping(url string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
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
