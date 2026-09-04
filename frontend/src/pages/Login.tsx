import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../store';

export function Login() {
  const nav = useNavigate();
  const setTokens = useAuth((s) => s.setTokens);
  const [login, setLogin] = useState('superadmin');
  const [password, setPassword] = useState('');
  const [err, setErr] = useState('');

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr('');
    try {
      const { data } = await api.post('/auth/login', { login, password });
      setTokens(data.access_token, data.refresh_token);
      nav('/me');
    } catch (e: any) {
      setErr(e.response?.data?.error ?? 'login failed');
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-100">
      <form onSubmit={submit} className="bg-white p-8 rounded shadow w-96">
        <h1 className="text-xl font-bold mb-4">RMS — вход</h1>
        <input
          className="w-full border rounded px-3 py-2 mb-2"
          placeholder="Логин или email"
          value={login}
          onChange={(e) => setLogin(e.target.value)}
        />
        <input
          className="w-full border rounded px-3 py-2 mb-2"
          placeholder="Пароль"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {err && <p className="text-red-600 text-sm mb-2">{err}</p>}
        <button className="w-full px-4 py-2 bg-slate-900 text-white rounded">Войти</button>
        <p className="text-sm mt-3 text-slate-500">
          Нет организации? <Link className="underline" to="/register">Регистрация</Link>
        </p>
      </form>
    </div>
  );
}
