import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Users() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['users'],
    queryFn: async () => (await api.get('/users')).data,
    retry: false,
  });

  return (
    <div className="p-8 max-w-3xl mx-auto">
      <h1 className="text-xl font-bold mb-4">Пользователи</h1>
      {isLoading && <p>Загрузка...</p>}
      {isError && <p className="text-red-600">Нет прав (нужен user:read) или нет сессии.</p>}
      {data && (
        <table className="w-full text-sm border">
          <thead>
            <tr className="bg-slate-100">
              <th className="border px-2 py-1">ID</th>
              <th className="border px-2 py-1">Логин</th>
              <th className="border px-2 py-1">Email</th>
              <th className="border px-2 py-1">Роли</th>
            </tr>
          </thead>
          <tbody>
            {data.map((u: any) => (
              <tr key={u.id}>
                <td className="border px-2 py-1">{u.id}</td>
                <td className="border px-2 py-1">{u.username}</td>
                <td className="border px-2 py-1">{u.email}</td>
                <td className="border px-2 py-1">{(u.roles ?? []).join(', ')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <Link className="underline text-sm mt-4 inline-block" to="/me">← Кабинет</Link>
    </div>
  );
}
