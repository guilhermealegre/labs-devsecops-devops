import { useState, useEffect } from 'react'

const API_URL = import.meta.env.VITE_API_URL || '/api'

function App() {
    const [data, setData] = useState([])
    const [stats, setStats] = useState(null)
    const [health, setHealth] = useState(null)
    const [loading, setLoading] = useState(true)
    const [lastUpdate, setLastUpdate] = useState(null)

    useEffect(() => {
        fetchAll()
        // Auto-refresh every 3 seconds for real-time feel
        const interval = setInterval(fetchAll, 3000)
        return () => clearInterval(interval)
    }, [])

    const fetchAll = async () => {
        try {
            const [dataRes, statsRes, healthRes] = await Promise.all([
                fetch(`${API_URL}/data`),
                fetch(`${API_URL}/stats`),
                fetch(`${API_URL}/health`)
            ])

            if (dataRes.ok) {
                const dataJson = await dataRes.json()
                setData(dataJson.data || [])
            }

            if (statsRes.ok) {
                const statsJson = await statsRes.json()
                setStats(statsJson)
            }

            if (healthRes.ok) {
                const healthJson = await healthRes.json()
                setHealth(healthJson)
            }

            setLastUpdate(new Date())
        } catch (error) {
            console.error('Failed to fetch data:', error)
        } finally {
            setLoading(false)
        }
    }

    const formatUptime = (seconds) => {
        const hours = Math.floor(seconds / 3600)
        const minutes = Math.floor((seconds % 3600) / 60)
        const secs = seconds % 60
        return `${hours}h ${minutes}m ${secs}s`
    }

    const formatTime = (date) => {
        return date.toLocaleTimeString('en-US', {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
        })
    }

    if (loading) {
        return (
            <div className="app">
                <div className="container">
                    <div className="loading">
                        <div className="spinner"></div>
                    </div>
                </div>
            </div>
        )
    }

    return (
        <div className="app">
            <div className="container">
                {/* Header */}
                <header className="header">
                    <div className="header-left">
                        <h1>Performance Dashboard</h1>
                        <p>App C - Rust + PostgreSQL High-Performance Stack</p>
                    </div>
                    <div className="header-right">
                        <div className="status-badge">
                            <span className="status-dot"></span>
                            <span>Live</span>
                        </div>
                        <button className="refresh-btn" onClick={fetchAll}>
                            ↻ Refresh
                        </button>
                        {lastUpdate && (
                            <span style={{ fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                                Last: {formatTime(lastUpdate)}
                            </span>
                        )}
                    </div>
                </header>

                {/* Stats Grid */}
                <div className="stats-grid">
                    <div className="stat-card">
                        <div className="stat-label">Total Records</div>
                        <div className="stat-value">{stats?.total_records?.toLocaleString() || 0}</div>
                    </div>
                    <div className="stat-card">
                        <div className="stat-label">Average Value</div>
                        <div className="stat-value">{stats?.avg_value?.toFixed(2) || 0}</div>
                    </div>
                    <div className="stat-card">
                        <div className="stat-label">Min Value</div>
                        <div className="stat-value">{stats?.min_value?.toFixed(2) || 0}</div>
                    </div>
                    <div className="stat-card">
                        <div className="stat-label">Max Value</div>
                        <div className="stat-value">{stats?.max_value?.toFixed(2) || 0}</div>
                    </div>
                </div>

                {/* Main Content */}
                <div className="content-grid">
                    {/* Data Table */}
                    <div className="card">
                        <div className="card-header">
                            <h2 className="card-title">Real-Time Data</h2>
                            <span style={{ fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                                {data.length} records
                            </span>
                        </div>

                        <div style={{ overflowX: 'auto' }}>
                            <table className="data-table">
                                <thead>
                                    <tr>
                                        <th>ID</th>
                                        <th>Timestamp</th>
                                        <th>Category</th>
                                        <th>Value</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {data.slice(0, 15).map((item) => (
                                        <tr key={item.id}>
                                            <td style={{ fontFamily: 'monospace', opacity: 0.7 }}>
                                                #{item.id}
                                            </td>
                                            <td style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>
                                                {new Date(item.timestamp).toLocaleString()}
                                            </td>
                                            <td>
                                                <span className="category-badge">{item.category}</span>
                                            </td>
                                            <td className="value-cell">
                                                {item.value.toFixed(4)}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    </div>

                    {/* Right Column */}
                    <div>
                        {/* Categories */}
                        <div className="card" style={{ marginBottom: '1.5rem' }}>
                            <div className="card-header">
                                <h2 className="card-title">Categories</h2>
                            </div>

                            <div className="category-list">
                                {stats?.categories?.map((cat) => (
                                    <div key={cat.category} className="category-item">
                                        <div className="category-info">
                                            <h4>{cat.category}</h4>
                                            <p>{cat.count} records</p>
                                        </div>
                                        <div className="category-value">
                                            {cat.avg_value.toFixed(2)}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>

                        {/* System Health */}
                        <div className="card">
                            <div className="card-header">
                                <h2 className="card-title">System Health</h2>
                            </div>

                            <div className="category-list">
                                <div className="category-item">
                                    <div className="category-info">
                                        <h4>Service Status</h4>
                                        <p>Backend API</p>
                                    </div>
                                    <div style={{
                                        color: health?.status === 'healthy' ? 'var(--success)' : 'var(--accent-secondary)',
                                        fontWeight: 600
                                    }}>
                                        {health?.status?.toUpperCase() || 'UNKNOWN'}
                                    </div>
                                </div>
                            </div>

                            {health?.uptime_seconds && (
                                <div className="uptime-section">
                                    <div className="uptime-label">Uptime</div>
                                    <div className="uptime-value">
                                        {formatUptime(health.uptime_seconds)}
                                    </div>
                                </div>
                            )}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default App
