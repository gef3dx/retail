import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../store';

export function Register() {
  const nav = useNavigate();
  const setTokens = useAuth((s) => s.setTokens);
  const [f, set] = useState({
    username: '',
    email: '',
    password: '',
    first_name: '',
    last_name: '',
    org_name: '',
    org_inn: '',
    org_kpp: '',
  });
  const [err, setErr] = useState('');

  function bind(k: keyof typeof f) {
    return {
      value: f[k],
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => set({ ...f, [k]: e.target.value }),
    };
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr('');
    try {
      const { data } = await api.post('/auth/register', { ...f });
      setTokens(data.access_token, data.refresh_token);
      nav('/me');
    } catch (e: any) {
      setErr(e.response?.data?.error ?? 'register failed');
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-100">
      <form onSubmit={submit} className="bg-white p-8 rounded shadow w-[28rem]">
        <h1 className="text-xl font-bold mb-4">Регистрация организации</h1>
        <div className="grid grid-cols-2 gap-2">
          <input className="border rounded px-3 py-2" placeholder="Логин" {...bind('username')} />
          <input className="border rounded px-3 py-2" placeholder="Email" {...bind('email')} />
          <input className="border rounded px-3 py-2" placeholder="Имя" {...bind('first_name')} />
          <input className="border rounded px-3 py-2" placeholder="Фамилия" {...bind('last_name')} />
          <input className="border rounded px-3 py-2 col-span-2" placeholder="Пароль (мин. 6)" type="password" {...bind('password')} />
          <input className="border rounded px-3 py-2 col-span-2" placeholder="Организация" {...bind('org_name')} />
          <input className="border rounded px-3 py-2" placeholder="ИНН" {...bind('org_inn')} />
          <input className="border rounded px-3 py-2" placeholder="КПП" {...bind('org_kpp')} />
        </div>
        {err && <p className="text-red-600 text-sm mt-2">{err}</p>}
        <button className="w-full mt-4 px-4 py-2 bg-slate-900 text-white rounded">Создать</button>
        <p className="text-sm mt-3 text-slate-500">
          Уже есть аккаунт? <Link className="underline" to="/login">Войти</Link>
        </p>
      </form>
    </div>
  );
}
