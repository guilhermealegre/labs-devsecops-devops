# =============================================================================
# App B Backend API - Python FastAPI with Prometheus Metrics
# =============================================================================
# Exposes:
#   - GET  /health      - Health check
#   - POST /jobs        - Submit a job to RabbitMQ
#   - GET  /jobs        - List job results from MongoDB
#   - GET  /metrics     - Prometheus metrics
# =============================================================================

import os
import json
import logging
from datetime import datetime
from typing import Optional
from contextlib import asynccontextmanager

import pika
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from starlette.responses import Response
from pymongo import MongoClient

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# =============================================================================
# Prometheus Metrics
# =============================================================================
http_requests_total = Counter(
    'http_requests_total',
    'Total HTTP requests',
    ['method', 'endpoint', 'status']
)

jobs_submitted_total = Counter(
    'jobs_submitted_total',
    'Total jobs submitted to the queue'
)

request_duration_seconds = Histogram(
    'request_duration_seconds',
    'Request duration in seconds',
    ['method', 'endpoint']
)

# =============================================================================
# Configuration
# =============================================================================
RABBITMQ_HOST = os.getenv('RABBITMQ_HOST', 'rabbitmq')
RABBITMQ_PORT = int(os.getenv('RABBITMQ_PORT', '5672'))
RABBITMQ_USER = os.getenv('RABBITMQ_USER', 'guest')
RABBITMQ_PASSWORD = os.getenv('RABBITMQ_PASSWORD', 'guest')
RABBITMQ_QUEUE = os.getenv('RABBITMQ_QUEUE', 'jobs')

MONGODB_URI = os.getenv('MONGODB_URI', 'mongodb://mongodb:27017')
MONGODB_DATABASE = os.getenv('MONGODB_DATABASE', 'app_b')

# =============================================================================
# RabbitMQ Connection
# =============================================================================
def get_rabbitmq_connection():
    """Create RabbitMQ connection with retry logic."""
    credentials = pika.PlainCredentials(RABBITMQ_USER, RABBITMQ_PASSWORD)
    parameters = pika.ConnectionParameters(
        host=RABBITMQ_HOST,
        port=RABBITMQ_PORT,
        credentials=credentials,
        heartbeat=600,
        blocked_connection_timeout=300
    )
    return pika.BlockingConnection(parameters)

# =============================================================================
# MongoDB Connection
# =============================================================================
mongo_client: Optional[MongoClient] = None
mongo_db = None

def get_mongodb():
    """Get MongoDB database connection."""
    global mongo_client, mongo_db
    if mongo_client is None:
        mongo_client = MongoClient(MONGODB_URI)
        mongo_db = mongo_client[MONGODB_DATABASE]
    return mongo_db

# =============================================================================
# Pydantic Models
# =============================================================================
class JobRequest(BaseModel):
    job_type: str
    payload: dict = {}

class JobResponse(BaseModel):
    id: str
    status: str
    message: str

# =============================================================================
# FastAPI App
# =============================================================================
@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan handler."""
    logger.info("Starting App B API...")
    
    # Wait for RabbitMQ to be ready
    import time
    for i in range(30):
        try:
            conn = get_rabbitmq_connection()
            channel = conn.channel()
            channel.queue_declare(queue=RABBITMQ_QUEUE, durable=True)
            conn.close()
            logger.info("Connected to RabbitMQ")
            break
        except Exception as e:
            logger.warning(f"Waiting for RabbitMQ... ({i+1}/30): {e}")
            time.sleep(1)
    
    yield
    
    # Cleanup
    global mongo_client
    if mongo_client:
        mongo_client.close()
    logger.info("Shutting down App B API...")

app = FastAPI(
    title="App B - Async Pipeline API",
    description="Python FastAPI backend for job processing",
    version="1.0.0",
    lifespan=lifespan
)

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# =============================================================================
# Routes
# =============================================================================
@app.get("/health")
async def health_check():
    """Health check endpoint."""
    http_requests_total.labels(method='GET', endpoint='/health', status='200').inc()
    return {"status": "healthy", "service": "app-b-api"}

@app.get("/metrics")
async def metrics():
    """Prometheus metrics endpoint."""
    return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)

@app.post("/jobs", response_model=JobResponse)
async def submit_job(job: JobRequest):
    """Submit a job to the RabbitMQ queue."""
    import time
    import uuid
    
    start_time = time.time()
    
    try:
        job_id = str(uuid.uuid4())
        
        message = {
            "id": job_id,
            "type": job.job_type,
            "payload": job.payload,
            "submitted_at": datetime.utcnow().isoformat()
        }
        
        # Publish to RabbitMQ
        connection = get_rabbitmq_connection()
        channel = connection.channel()
        channel.queue_declare(queue=RABBITMQ_QUEUE, durable=True)
        
        channel.basic_publish(
            exchange='',
            routing_key=RABBITMQ_QUEUE,
            body=json.dumps(message),
            properties=pika.BasicProperties(
                delivery_mode=2,  # Persistent
                content_type='application/json'
            )
        )
        
        connection.close()
        
        # Update metrics
        jobs_submitted_total.inc()
        http_requests_total.labels(method='POST', endpoint='/jobs', status='202').inc()
        request_duration_seconds.labels(method='POST', endpoint='/jobs').observe(time.time() - start_time)
        
        logger.info(f"Job {job_id} submitted successfully")
        
        return JobResponse(
            id=job_id,
            status="queued",
            message="Job submitted successfully"
        )
        
    except Exception as e:
        http_requests_total.labels(method='POST', endpoint='/jobs', status='500').inc()
        logger.error(f"Failed to submit job: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/jobs")
async def list_jobs():
    """List all processed jobs from MongoDB."""
    import time
    
    start_time = time.time()
    
    try:
        db = get_mongodb()
        jobs = list(db.jobs.find({}).sort("processed_at", -1).limit(50))
        
        # Convert ObjectId to string
        for job in jobs:
            job['_id'] = str(job['_id'])
        
        http_requests_total.labels(method='GET', endpoint='/jobs', status='200').inc()
        request_duration_seconds.labels(method='GET', endpoint='/jobs').observe(time.time() - start_time)
        
        return {"jobs": jobs, "count": len(jobs)}
        
    except Exception as e:
        http_requests_total.labels(method='GET', endpoint='/jobs', status='500').inc()
        logger.error(f"Failed to list jobs: {e}")
        raise HTTPException(status_code=500, detail=str(e))

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8080)
