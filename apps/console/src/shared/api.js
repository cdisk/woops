import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

api.interceptors.response.use(
  (r) => r,
  (err) => {
    const status = err.response?.status
    // 401 = unauthenticated; also treat bare 403 without body as session loss (legacy).
    if (status === 401 || (status === 403 && err.response?.data?.error === 'unauthorized')) {
      localStorage.removeItem('token')
      if (location.pathname !== '/login') location.href = '/login'
    }
    return Promise.reject(err)
  }
)

export default api
