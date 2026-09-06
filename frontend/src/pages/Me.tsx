import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../store';

export function Me() {
  const logout = useAuth((s) => s.logout);
  const nav = useNavigate();
  const qc = useQueryClient();
  const [tg, setTg] = useState<string | null>(null);
  const [push, setPush] = useState<string | null>(null);
  const [saved, setSaved] = useState('');
  const { data, isLoading, isError } = useQuery({
    queryKey: ['me'],
    queryFn: async () => (await api.get('/me')).data,
    retry: false,
  });

  const save = useMutation({
    mutationFn: async () =>
      (await api.patch('/me', { telegram_chat_id: tg, push_token: push })).data,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['me'] }); setSaved('Сохранено'); },
    onError: () => setSaved('Ошибка'),
  });

  if (isLoading) return <p className="p-8">Загрузка...</p>;
  if (isError) return <p className="p-8">Нет доступа. <Link className="underline" to="/login">Войти</Link></p>;

  return (
    <div className="p-8 max-w-xl mx-auto">
      <h1 className="text-xl font-bold mb-2">Кабинет: {data.username}</h1>
      <pre className="text-xs bg-slate-50 border rounded p-3">{JSON.stringify(data, null, 2)}</pre>
      <div className="border rounded p-3 mt-3">
        <h2 className="font-bold text-sm mb-2">Адреса уведомлений</h2>
        <div className="flex gap-2 text-sm">
          <label className="flex-1">Telegram chat id
            <input className="border rounded px-2 py-1 w-full" placeholder={data.telegram_chat_id ?? 'не задан'}
              value={tg ?? ''} onChange={(e) => setTg(e.target.value)} />
          </label>
          <label className="flex-1">Push token
            <input className="border rounded px-2 py-1 w-full" placeholder={data.push_token ?? 'не задан'}
              value={push ?? ''} onChange={(e) => setPush(e.target.value)} />
          </label>
          <button className="px-3 self-end py-1 bg-slate-900 text-white rounded"
            onClick={() => save.mutate()}>OK</button>
        </div>
        {saved && <p className="text-xs mt-1">{saved}</p>}
      </div>
      <div className="flex gap-2 mt-4 flex-wrap">
        <Link className="px-4 py-2 border rounded" to="/users">Пользователи</Link>
        <Link className="px-4 py-2 border rounded" to="/orgs">Организации</Link>
        <Link className="px-4 py-2 border rounded" to="/products">Товары</Link>
        <Link className="px-4 py-2 border rounded" to="/dicts">Справочники</Link>
        <Link className="px-4 py-2 border rounded" to="/cashier">Касса</Link>
        <Link className="px-4 py-2 border rounded" to="/marking">Маркировка</Link>
        <Link className="px-4 py-2 border rounded" to="/stock">Склад</Link>
        <Link className="px-4 py-2 border rounded" to="/orders">Заказы</Link>
        <Link className="px-4 py-2 border rounded" to="/notify">Уведомления</Link>
        <Link className="px-4 py-2 border rounded" to="/integrations">Интеграции</Link>
        <Link className="px-4 py-2 border rounded" to="/services">Услуги</Link>
        <Link className="px-4 py-2 border rounded" to="/bookings">Брони</Link>
        <Link className="px-4 py-2 border rounded" to="/delivery">Доставка</Link>
        <Link className="px-4 py-2 border rounded" to="/marketplaces">Маркеты</Link>
        <Link className="px-4 py-2 border rounded" to="/egais">ЕГАИС</Link>
        <Link className="px-4 py-2 border rounded" to="/reports">Отчёты</Link>
        <button
          className="px-4 py-2 bg-slate-900 text-white rounded"
          onClick={() => {
            logout();
            nav('/login');
          }}
        >
          Выйти
        </button>
      </div>
    </div>
  );
}
