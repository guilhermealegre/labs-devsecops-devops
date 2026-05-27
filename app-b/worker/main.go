// =============================================================================
// App B Worker - Go RabbitMQ Consumer with Prometheus Metrics
// =============================================================================
// Consumes jobs from RabbitMQ queue and saves results to MongoDB.
//
// Exposes:
//   - GET /health   - Health check
//   - GET /metrics  - Prometheus metrics
// =============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Prometheus metrics
var (
	jobsProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobs_processed_total",
			Help: "Total number of jobs processed",
		},
		[]string{"status", "job_type"},
	)

	jobProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "job_processing_duration_seconds",
			Help:    "Duration of job processing in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"job_type"},
	)

	jobsInProgress = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "jobs_in_progress",
			Help: "Number of jobs currently being processed",
		},
	)

	rabbitMQConnected = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rabbitmq_connected",
			Help: "Whether the worker is connected to RabbitMQ (1=connected, 0=disconnected)",
		},
	)
)

func init() {
	prometheus.MustRegister(jobsProcessedTotal)
	prometheus.MustRegister(jobProcessingDuration)
	prometheus.MustRegister(jobsInProgress)
	prometheus.MustRegister(rabbitMQConnected)
}

// Job represents a job message from the queue
type Job struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
	SubmittedAt string                 `json:"submitted_at"`
}

// JobResult represents the processed job result
type JobResult struct {
	JobID       string                 `json:"job_id"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
	Result      map[string]interface{} `json:"result"`
	Status      string                 `json:"status"`
	ProcessedAt time.Time              `json:"processed_at"`
	Duration    float64                `json:"duration_seconds"`
}

// Configuration
var (
	rabbitMQURL  string
	mongoDBURI   string
	queueName    string
	databaseName string
	metricsPort  string
)

func init() {
	rabbitMQURL = getEnv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	mongoDBURI = getEnv("MONGODB_URI", "mongodb://mongodb:27017")
	queueName = getEnv("RABBITMQ_QUEUE", "jobs")
	databaseName = getEnv("MONGODB_DATABASE", "app_b")
	metricsPort = getEnv("METRICS_PORT", "8080")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	log.Println("Starting App B Worker...")

	// Start metrics server
	go startMetricsServer()

	// Connect to MongoDB
	mongoClient, err := connectMongoDB()
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	collection := mongoClient.Database(databaseName).Collection("jobs")

	// Connect to RabbitMQ with retry
	conn, channel, err := connectRabbitMQ()
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()
	defer channel.Close()

	rabbitMQConnected.Set(1)

	// Declare queue
	q, err := channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %v", err)
	}

	// Set QoS
	err = channel.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		log.Fatalf("Failed to set QoS: %v", err)
	}

	// Start consuming
	msgs, err := channel.Consume(
		q.Name,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		log.Fatalf("Failed to register consumer: %v", err)
	}

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Worker started. Waiting for jobs on queue: %s", queueName)

	for {
		select {
		case <-sigChan:
			log.Println("Shutdown signal received. Exiting...")
			return
		case msg, ok := <-msgs:
			if !ok {
				log.Println("Channel closed. Attempting to reconnect...")
				rabbitMQConnected.Set(0)
				conn, channel, err = connectRabbitMQ()
				if err != nil {
					log.Fatalf("Failed to reconnect to RabbitMQ: %v", err)
				}
				rabbitMQConnected.Set(1)
				continue
			}
			processJob(msg, collection)
		}
	}
}

func startMetricsServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})
	mux.Handle("/metrics", promhttp.Handler())

	log.Printf("Metrics server starting on port %s", metricsPort)
	if err := http.ListenAndServe(":"+metricsPort, mux); err != nil {
		log.Fatalf("Metrics server failed: %v", err)
	}
}

func connectMongoDB() (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var client *mongo.Client
	var err error

	for i := 0; i < 30; i++ {
		client, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoDBURI))
		if err == nil {
			err = client.Ping(ctx, nil)
			if err == nil {
				log.Println("Connected to MongoDB")
				return client, nil
			}
		}
		log.Printf("Waiting for MongoDB... (%d/30)", i+1)
		time.Sleep(time.Second)
	}

	return nil, fmt.Errorf("failed to connect to MongoDB after 30 attempts: %v", err)
}

func connectRabbitMQ() (*amqp.Connection, *amqp.Channel, error) {
	var conn *amqp.Connection
	var channel *amqp.Channel
	var err error

	for i := 0; i < 30; i++ {
		conn, err = amqp.Dial(rabbitMQURL)
		if err == nil {
			channel, err = conn.Channel()
			if err == nil {
				log.Println("Connected to RabbitMQ")
				return conn, channel, nil
			}
			conn.Close()
		}
		log.Printf("Waiting for RabbitMQ... (%d/30)", i+1)
		time.Sleep(time.Second)
	}

	return nil, nil, fmt.Errorf("failed to connect to RabbitMQ after 30 attempts: %v", err)
}

func processJob(msg amqp.Delivery, collection *mongo.Collection) {
	startTime := time.Now()
	jobsInProgress.Inc()
	defer jobsInProgress.Dec()

	var job Job
	if err := json.Unmarshal(msg.Body, &job); err != nil {
		log.Printf("Failed to parse job: %v", err)
		msg.Nack(false, false)
		jobsProcessedTotal.WithLabelValues("error", "unknown").Inc()
		return
	}

	log.Printf("Processing job: %s (type: %s)", job.ID, job.Type)

	// Simulate job processing based on type
	result := processJobByType(job)

	// Calculate duration
	duration := time.Since(startTime).Seconds()

	// Create job result
	jobResult := JobResult{
		JobID:       job.ID,
		Type:        job.Type,
		Payload:     job.Payload,
		Result:      result,
		Status:      "completed",
		ProcessedAt: time.Now(),
		Duration:    duration,
	}

	// Save to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, jobResult)
	if err != nil {
		log.Printf("Failed to save job result: %v", err)
		msg.Nack(false, true)
		jobsProcessedTotal.WithLabelValues("error", job.Type).Inc()
		return
	}

	// Acknowledge message
	msg.Ack(false)

	// Update metrics
	jobsProcessedTotal.WithLabelValues("success", job.Type).Inc()
	jobProcessingDuration.WithLabelValues(job.Type).Observe(duration)

	log.Printf("Job %s completed in %.3fs", job.ID, duration)
}

func processJobByType(job Job) map[string]interface{} {
	result := make(map[string]interface{})

	switch job.Type {
	case "process":
		// Simulate data processing
		time.Sleep(100 * time.Millisecond)
		result["processed"] = true
		result["items_processed"] = 42
		result["message"] = "Data processed successfully"

	case "analyze":
		// Simulate analysis
		time.Sleep(200 * time.Millisecond)
		result["analyzed"] = true
		result["score"] = 0.95
		result["message"] = "Analysis completed"

	case "transform":
		// Simulate transformation
		time.Sleep(150 * time.Millisecond)
		result["transformed"] = true
		result["original_size"] = 1024
		result["final_size"] = 512
		result["message"] = "Transformation applied"

	default:
		// Generic processing
		time.Sleep(50 * time.Millisecond)
		result["processed"] = true
		result["message"] = "Generic job completed"
	}

	return result
}
