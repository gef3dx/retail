import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { useAuth } from '../store';

export function Me() {
  const logout = useAuth((s) => s.logout);
  const nav = useNavigate();
  const { data, isLoading, isError } = useQuery({
    queryKey: ['me'],
    queryFn: async () => (await api.get('/me')).data,
    retry: false,
  });

  if (isLoading) return <p className="p-8">Загрузка...</p>;
  if (isError) return <p className="p-8">Нет доступа. <Link className="underline" to="/login">Войти</Link></p>;

  return (
    <div className="p-8 max-w-xl mx-auto">
      <h1 className="text-xl font-bold mb-2">Кабинет: {data.username}</h1>
      <pre className="text-xs bg-slate-50 border rounded p-3">{JSON.stringify(data, null, 2)}</pre>
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
