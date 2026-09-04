import axios from 'axios';

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL ?? '/api/v1',
  timeout: 8000,
});

api.interceptors.request.use((cfg) => {
  const token = localStorage.getItem('rms_access') ?? localStorage.getItem('rms_token');
  if (token) cfg.headers.Authorization = `Bearer ${token}`;
  return cfg;
});

// Одна попытка refresh при 401, затем разлогин.
let refreshing = false;
api.interceptors.response.use(
  (r) => r,
  async (err) => {
    const orig = err.config;
    if (err.response?.status === 401 && !orig?._retried && localStorage.getItem('rms_refresh')) {
      orig._retried = true;
      if (refreshing) return Promise.reject(err);
      refreshing = true;
      try {
        const { data } = await axios.post(
          `${api.defaults.baseURL}/auth/refresh`,
          { refresh_token: localStorage.getItem('rms_refresh') },
        );
        localStorage.setItem('rms_access', data.access_token);
        localStorage.setItem('rms_refresh', data.refresh_token);
        orig.headers.Authorization = `Bearer ${data.access_token}`;
        return api(orig);
      } catch {
        localStorage.removeItem('rms_access');
        localStorage.removeItem('rms_refresh');
        location.href = '/login';
      } finally {
        refreshing = false;
      }
    }
    return Promise.reject(err);
  },
);
