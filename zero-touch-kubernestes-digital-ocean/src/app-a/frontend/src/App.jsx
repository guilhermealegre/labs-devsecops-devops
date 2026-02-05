import { useState, useEffect } from 'react'

// API URL - use environment variable in production
const API_URL = import.meta.env.VITE_API_URL || '/api'

function App() {
    const [users, setUsers] = useState([])
    const [loading, setLoading] = useState(true)
    const [submitting, setSubmitting] = useState(false)
    const [message, setMessage] = useState(null)
    const [formData, setFormData] = useState({ name: '', email: '' })

    // Fetch users on component mount
    useEffect(() => {
        fetchUsers()
    }, [])

    const fetchUsers = async () => {
        try {
            setLoading(true)
            const response = await fetch(`${API_URL}/users`)
            if (!response.ok) throw new Error('Failed to fetch users')
            const data = await response.json()
            setUsers(data || [])
        } catch (error) {
            setMessage({ type: 'error', text: 'Failed to load users' })
        } finally {
            setLoading(false)
        }
    }

    const handleSubmit = async (e) => {
        e.preventDefault()

        if (!formData.name || !formData.email) {
            setMessage({ type: 'error', text: 'Please fill in all fields' })
            return
        }

        try {
            setSubmitting(true)
            const response = await fetch(`${API_URL}/users`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(formData)
            })

            if (!response.ok) throw new Error('Failed to create user')

            const newUser = await response.json()
            setUsers([newUser, ...users])
            setFormData({ name: '', email: '' })
            setMessage({ type: 'success', text: 'User created successfully!' })

            // Clear message after 3 seconds
            setTimeout(() => setMessage(null), 3000)
        } catch (error) {
            setMessage({ type: 'error', text: 'Failed to create user' })
        } finally {
            setSubmitting(false)
        }
    }

    const formatDate = (dateString) => {
        return new Date(dateString).toLocaleDateString('en-US', {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        })
    }

    const getInitials = (name) => {
        return name.split(' ').map(n => n[0]).join('').slice(0, 2)
    }

    return (
        <div className="app">
            <div className="container">
                <header className="header">
                    <h1>User Management</h1>
                    <p>App A - Go + PostgreSQL Stack</p>
                    <div className="badge">
                        <span>Connected to Backend</span>
                    </div>
                </header>

                <div className="grid">
                    {/* Add User Form */}
                    <div className="card">
                        <h2 className="card-title">Add New User</h2>

                        {message && (
                            <div className={`message ${message.type}`}>
                                {message.text}
                            </div>
                        )}

                        <form onSubmit={handleSubmit}>
                            <div className="form-group">
                                <label htmlFor="name">Full Name</label>
                                <input
                                    type="text"
                                    id="name"
                                    placeholder="John Doe"
                                    value={formData.name}
                                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                                    disabled={submitting}
                                />
                            </div>

                            <div className="form-group">
                                <label htmlFor="email">Email Address</label>
                                <input
                                    type="email"
                                    id="email"
                                    placeholder="john@example.com"
                                    value={formData.email}
                                    onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                                    disabled={submitting}
                                />
                            </div>

                            <button type="submit" className="btn" disabled={submitting}>
                                {submitting ? 'Creating...' : 'Create User'}
                            </button>
                        </form>
                    </div>

                    {/* User List */}
                    <div className="card">
                        <h2 className="card-title">Users</h2>

                        <div className="stats">
                            <div className="stat">
                                <div className="stat-value">{users.length}</div>
                                <div className="stat-label">Total Users</div>
                            </div>
                        </div>

                        {loading ? (
                            <div className="loading">
                                <div className="spinner"></div>
                            </div>
                        ) : users.length === 0 ? (
                            <div className="empty-state">
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                                    <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                                    <circle cx="9" cy="7" r="4" />
                                    <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                                    <path d="M16 3.13a4 4 0 0 1 0 7.75" />
                                </svg>
                                <p>No users yet. Add your first user!</p>
                            </div>
                        ) : (
                            <div className="user-list">
                                {users.map((user) => (
                                    <div key={user.id} className="user-item">
                                        <div className="user-avatar">
                                            {getInitials(user.name)}
                                        </div>
                                        <div className="user-info">
                                            <div className="user-name">{user.name}</div>
                                            <div className="user-email">{user.email}</div>
                                        </div>
                                        <div className="user-date">
                                            {formatDate(user.created_at)}
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
