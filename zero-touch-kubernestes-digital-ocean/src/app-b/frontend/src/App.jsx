import { useState, useEffect } from 'react'

const API_URL = import.meta.env.VITE_API_URL || '/api'

const JOB_TYPES = [
    { id: 'process', name: 'Process Data', description: 'Transform and process incoming data' },
    { id: 'analyze', name: 'Analyze', description: 'Run analytical computations' },
    { id: 'transform', name: 'Transform', description: 'Apply data transformations' },
]

function App() {
    const [jobs, setJobs] = useState([])
    const [loading, setLoading] = useState(true)
    const [submitting, setSubmitting] = useState(false)
    const [message, setMessage] = useState(null)
    const [selectedType, setSelectedType] = useState('process')

    useEffect(() => {
        fetchJobs()
        // Poll for updates every 5 seconds
        const interval = setInterval(fetchJobs, 5000)
        return () => clearInterval(interval)
    }, [])

    const fetchJobs = async () => {
        try {
            const response = await fetch(`${API_URL}/jobs`)
            if (!response.ok) throw new Error('Failed to fetch jobs')
            const data = await response.json()
            setJobs(data.jobs || [])
        } catch (error) {
            console.error('Failed to fetch jobs:', error)
        } finally {
            setLoading(false)
        }
    }

    const submitJob = async () => {
        try {
            setSubmitting(true)
            const response = await fetch(`${API_URL}/jobs`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    job_type: selectedType,
                    payload: {
                        timestamp: new Date().toISOString(),
                        source: 'web-ui'
                    }
                })
            })

            if (!response.ok) throw new Error('Failed to submit job')

            const result = await response.json()
            setMessage({ type: 'success', text: `Job ${result.id.slice(0, 8)} submitted!` })

            // Refresh jobs after a short delay
            setTimeout(fetchJobs, 1000)
            setTimeout(() => setMessage(null), 3000)
        } catch (error) {
            setMessage({ type: 'error', text: 'Failed to submit job' })
        } finally {
            setSubmitting(false)
        }
    }

    const formatDate = (dateString) => {
        return new Date(dateString).toLocaleString('en-US', {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
        })
    }

    const getJobIcon = (type) => {
        switch (type) {
            case 'process': return '⚙️'
            case 'analyze': return '📊'
            case 'transform': return '🔄'
            default: return '📦'
        }
    }

    const completedJobs = jobs.filter(j => j.status === 'completed').length
    const avgDuration = jobs.length > 0
        ? (jobs.reduce((sum, j) => sum + (j.duration_seconds || 0), 0) / jobs.length).toFixed(3)
        : 0

    return (
        <div className="app">
            <div className="container">
                <header className="header">
                    <h1>Async Job Pipeline</h1>
                    <p>App B - Python + RabbitMQ + Go Worker + MongoDB</p>

                    <div className="pipeline">
                        <div className="pipeline-step active">
                            <span>React</span>
                        </div>
                        <span className="pipeline-arrow">→</span>
                        <div className="pipeline-step active">
                            <span>Python API</span>
                        </div>
                        <span className="pipeline-arrow">→</span>
                        <div className="pipeline-step active">
                            <span>RabbitMQ</span>
                        </div>
                        <span className="pipeline-arrow">→</span>
                        <div className="pipeline-step active">
                            <span>Go Worker</span>
                        </div>
                        <span className="pipeline-arrow">→</span>
                        <div className="pipeline-step active">
                            <span>MongoDB</span>
                        </div>
                    </div>
                </header>

                <div className="grid">
                    {/* Job Submission */}
                    <div className="card">
                        <h2 className="card-title">Submit Job</h2>

                        {message && (
                            <div className={`message ${message.type}`}>
                                {message.text}
                            </div>
                        )}

                        <div className="job-types">
                            {JOB_TYPES.map((type) => (
                                <div
                                    key={type.id}
                                    className={`job-type ${selectedType === type.id ? 'selected' : ''}`}
                                    onClick={() => setSelectedType(type.id)}
                                >
                                    <div className="job-type-radio"></div>
                                    <div className="job-type-info">
                                        <h4>{type.name}</h4>
                                        <p>{type.description}</p>
                                    </div>
                                </div>
                            ))}
                        </div>

                        <button
                            className="btn"
                            onClick={submitJob}
                            disabled={submitting}
                        >
                            {submitting ? 'Submitting...' : 'Process Job'}
                        </button>
                    </div>

                    {/* Job Results */}
                    <div className="card">
                        <h2 className="card-title">Processed Jobs</h2>

                        <div className="stats">
                            <div className="stat">
                                <div className="stat-value">{jobs.length}</div>
                                <div className="stat-label">Total Jobs</div>
                            </div>
                            <div className="stat">
                                <div className="stat-value">{completedJobs}</div>
                                <div className="stat-label">Completed</div>
                            </div>
                            <div className="stat">
                                <div className="stat-value">{avgDuration}s</div>
                                <div className="stat-label">Avg Duration</div>
                            </div>
                        </div>

                        {loading ? (
                            <div className="loading">
                                <div className="spinner"></div>
                            </div>
                        ) : jobs.length === 0 ? (
                            <div className="empty-state">
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                                    <path d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
                                </svg>
                                <p>No jobs processed yet. Submit your first job!</p>
                            </div>
                        ) : (
                            <div className="job-list">
                                {jobs.map((job) => (
                                    <div key={job._id} className="job-item">
                                        <div className="job-icon">
                                            {getJobIcon(job.type)}
                                        </div>
                                        <div className="job-info">
                                            <div className="job-header">
                                                <span className="job-id">{job.job_id?.slice(0, 8)}...</span>
                                                <span className="job-type-badge">{job.type}</span>
                                                <span className={`job-status ${job.status}`}>{job.status}</span>
                                            </div>
                                            {job.result && (
                                                <div className="job-result">
                                                    {job.result.message}
                                                </div>
                                            )}
                                            <div className="job-meta">
                                                <span>Duration: {job.duration_seconds?.toFixed(3)}s</span>
                                                <span>•</span>
                                                <span>{formatDate(job.processed_at)}</span>
                                            </div>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </div>
            </div>
        </div>
    )
}

export default App
