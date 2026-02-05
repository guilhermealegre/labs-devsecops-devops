//! =============================================================================
//! App C Backend - Rust High-Performance API with Prometheus Metrics
//! =============================================================================
//! A high-performance API built with Axum and tokio-postgres.
//!
//! Exposes:
//!   - GET  /health      - Health check
//!   - GET  /data        - Fetch real-time data
//!   - GET  /stats       - Get system statistics
//!   - GET  /metrics     - Prometheus metrics
//! =============================================================================

use axum::{
    extract::State,
    http::StatusCode,
    response::Json,
    routing::get,
    Router,
};
use prometheus::{
    Encoder, Gauge, GaugeVec, Histogram, HistogramOpts, HistogramVec, Opts, Registry, TextEncoder,
};
use serde::{Deserialize, Serialize};
use std::{
    env,
    net::SocketAddr,
    sync::Arc,
    time::{Duration, Instant},
};
use tokio::sync::RwLock;
use tokio_postgres::{Client, NoTls};
use tower_http::cors::{Any, CorsLayer};

// =============================================================================
// Application State
// =============================================================================

struct AppState {
    db: Arc<RwLock<Client>>,
    metrics: Metrics,
    start_time: Instant,
}

struct Metrics {
    registry: Registry,
    active_connections: Gauge,
    thread_usage: GaugeVec,
    request_duration: HistogramVec,
    requests_total: GaugeVec,
    db_query_duration: Histogram,
}

impl Metrics {
    fn new() -> Self {
        let registry = Registry::new();

        let active_connections = Gauge::new(
            "active_connections",
            "Number of active database connections",
        )
        .unwrap();

        let thread_usage = GaugeVec::new(
            Opts::new("thread_usage", "Thread pool usage metrics"),
            &["type"],
        )
        .unwrap();

        let request_duration = HistogramVec::new(
            HistogramOpts::new("request_duration_seconds", "HTTP request duration in seconds"),
            &["method", "endpoint"],
        )
        .unwrap();

        let requests_total = GaugeVec::new(
            Opts::new("requests_total", "Total number of requests"),
            &["endpoint", "status"],
        )
        .unwrap();

        let db_query_duration = Histogram::with_opts(HistogramOpts::new(
            "db_query_duration_seconds",
            "Database query duration in seconds",
        ))
        .unwrap();

        registry.register(Box::new(active_connections.clone())).unwrap();
        registry.register(Box::new(thread_usage.clone())).unwrap();
        registry.register(Box::new(request_duration.clone())).unwrap();
        registry.register(Box::new(requests_total.clone())).unwrap();
        registry.register(Box::new(db_query_duration.clone())).unwrap();

        Self {
            registry,
            active_connections,
            thread_usage,
            request_duration,
            requests_total,
            db_query_duration,
        }
    }
}

// =============================================================================
// Data Models
// =============================================================================

#[derive(Serialize)]
struct HealthResponse {
    status: String,
    service: String,
    uptime_seconds: u64,
}

#[derive(Serialize)]
struct DataPoint {
    id: i32,
    timestamp: String,
    value: f64,
    category: String,
}

#[derive(Serialize)]
struct DataResponse {
    data: Vec<DataPoint>,
    count: usize,
    generated_at: String,
}

#[derive(Serialize)]
struct StatsResponse {
    total_records: i64,
    avg_value: f64,
    min_value: f64,
    max_value: f64,
    categories: Vec<CategoryStat>,
}

#[derive(Serialize)]
struct CategoryStat {
    category: String,
    count: i64,
    avg_value: f64,
}

// =============================================================================
// Main
// =============================================================================

#[tokio::main]
async fn main() {
    // Initialize tracing
    tracing_subscriber::init();

    // Get configuration from environment
    let db_host = env::var("DB_HOST").unwrap_or_else(|_| "postgres".to_string());
    let db_port = env::var("DB_PORT").unwrap_or_else(|_| "5432".to_string());
    let db_user = env::var("DB_USER").unwrap_or_else(|_| "postgres".to_string());
    let db_password = env::var("DB_PASSWORD").unwrap_or_else(|_| "postgres".to_string());
    let db_name = env::var("DB_NAME").unwrap_or_else(|_| "app_c".to_string());
    let port: u16 = env::var("PORT")
        .unwrap_or_else(|_| "8080".to_string())
        .parse()
        .unwrap();

    let conn_str = format!(
        "host={} port={} user={} password={} dbname={}",
        db_host, db_port, db_user, db_password, db_name
    );

    // Connect to database with retry
    let client = connect_db(&conn_str).await;
    
    // Initialize database schema
    init_db(&client).await;

    // Create application state
    let metrics = Metrics::new();
    let state = Arc::new(AppState {
        db: Arc::new(RwLock::new(client)),
        metrics,
        start_time: Instant::now(),
    });

    // Update thread metrics periodically
    let state_clone = state.clone();
    tokio::spawn(async move {
        loop {
            update_thread_metrics(&state_clone);
            tokio::time::sleep(Duration::from_secs(5)).await;
        }
    });

    // Build router
    let app = Router::new()
        .route("/health", get(health_handler))
        .route("/data", get(data_handler))
        .route("/stats", get(stats_handler))
        .route("/metrics", get(metrics_handler))
        .with_state(state)
        .layer(CorsLayer::new().allow_origin(Any).allow_methods(Any));

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    tracing::info!("Server starting on {}", addr);

    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

// =============================================================================
// Database Functions
// =============================================================================

async fn connect_db(conn_str: &str) -> Client {
    for i in 0..30 {
        match tokio_postgres::connect(conn_str, NoTls).await {
            Ok((client, connection)) => {
                tokio::spawn(async move {
                    if let Err(e) = connection.await {
                        eprintln!("Database connection error: {}", e);
                    }
                });
                tracing::info!("Connected to PostgreSQL");
                return client;
            }
            Err(e) => {
                tracing::warn!("Waiting for database... ({}/30): {}", i + 1, e);
                tokio::time::sleep(Duration::from_secs(1)).await;
            }
        }
    }
    panic!("Failed to connect to database after 30 attempts");
}

async fn init_db(client: &Client) {
    client
        .batch_execute(
            r#"
            CREATE TABLE IF NOT EXISTS data_points (
                id SERIAL PRIMARY KEY,
                timestamp TIMESTAMPTZ DEFAULT NOW(),
                value DOUBLE PRECISION NOT NULL,
                category VARCHAR(50) NOT NULL
            );

            -- Insert sample data if empty
            INSERT INTO data_points (value, category)
            SELECT 
                random() * 100,
                (ARRAY['performance', 'reliability', 'throughput', 'latency'])[floor(random() * 4 + 1)::int]
            FROM generate_series(1, 100)
            WHERE NOT EXISTS (SELECT 1 FROM data_points LIMIT 1);
            "#,
        )
        .await
        .expect("Failed to initialize database");
    
    tracing::info!("Database initialized");
}

fn update_thread_metrics(state: &AppState) {
    // Simulate thread metrics
    let num_cpus = num_cpus::get() as f64;
    state.metrics.thread_usage.with_label_values(&["active"]).set(num_cpus * 0.6);
    state.metrics.thread_usage.with_label_values(&["idle"]).set(num_cpus * 0.4);
    state.metrics.thread_usage.with_label_values(&["total"]).set(num_cpus);
}

// =============================================================================
// Handlers
// =============================================================================

async fn health_handler(State(state): State<Arc<AppState>>) -> Json<HealthResponse> {
    let uptime = state.start_time.elapsed().as_secs();
    Json(HealthResponse {
        status: "healthy".to_string(),
        service: "app-c-backend".to_string(),
        uptime_seconds: uptime,
    })
}

async fn data_handler(State(state): State<Arc<AppState>>) -> Result<Json<DataResponse>, StatusCode> {
    let start = Instant::now();
    
    let db = state.db.read().await;
    
    state.metrics.active_connections.inc();
    
    let rows = db
        .query(
            "SELECT id, timestamp, value, category FROM data_points ORDER BY timestamp DESC LIMIT 50",
            &[],
        )
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;

    state.metrics.active_connections.dec();
    
    let duration = start.elapsed().as_secs_f64();
    state.metrics.db_query_duration.observe(duration);
    state.metrics.request_duration
        .with_label_values(&["GET", "/data"])
        .observe(duration);
    state.metrics.requests_total
        .with_label_values(&["/data", "200"])
        .inc();

    let data: Vec<DataPoint> = rows
        .iter()
        .map(|row| {
            let timestamp: chrono::DateTime<chrono::Utc> = row.get(1);
            DataPoint {
                id: row.get(0),
                timestamp: timestamp.to_rfc3339(),
                value: row.get(2),
                category: row.get(3),
            }
        })
        .collect();

    let count = data.len();

    Ok(Json(DataResponse {
        data,
        count,
        generated_at: chrono::Utc::now().to_rfc3339(),
    }))
}

async fn stats_handler(State(state): State<Arc<AppState>>) -> Result<Json<StatsResponse>, StatusCode> {
    let start = Instant::now();
    
    let db = state.db.read().await;
    
    state.metrics.active_connections.inc();

    // Get aggregate stats
    let stats_row = db
        .query_one(
            "SELECT COUNT(*), AVG(value), MIN(value), MAX(value) FROM data_points",
            &[],
        )
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;

    // Get category stats
    let category_rows = db
        .query(
            "SELECT category, COUNT(*), AVG(value) FROM data_points GROUP BY category ORDER BY COUNT(*) DESC",
            &[],
        )
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;

    state.metrics.active_connections.dec();
    
    let duration = start.elapsed().as_secs_f64();
    state.metrics.db_query_duration.observe(duration);
    state.metrics.request_duration
        .with_label_values(&["GET", "/stats"])
        .observe(duration);
    state.metrics.requests_total
        .with_label_values(&["/stats", "200"])
        .inc();

    let categories: Vec<CategoryStat> = category_rows
        .iter()
        .map(|row| CategoryStat {
            category: row.get(0),
            count: row.get(1),
            avg_value: row.get(2),
        })
        .collect();

    Ok(Json(StatsResponse {
        total_records: stats_row.get(0),
        avg_value: stats_row.get::<_, Option<f64>>(1).unwrap_or(0.0),
        min_value: stats_row.get::<_, Option<f64>>(2).unwrap_or(0.0),
        max_value: stats_row.get::<_, Option<f64>>(3).unwrap_or(0.0),
        categories,
    }))
}

async fn metrics_handler(State(state): State<Arc<AppState>>) -> String {
    let encoder = TextEncoder::new();
    let metric_families = state.metrics.registry.gather();
    let mut buffer = Vec::new();
    encoder.encode(&metric_families, &mut buffer).unwrap();
    String::from_utf8(buffer).unwrap()
}
