import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Orgs() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['orgs'],
    queryFn: async () => (await api.get('/organizations')).data,
    retry: false,
  });

  return (
    <div className="p-8 max-w-3xl mx-auto">
      <h1 className="text-xl font-bold mb-4">Организации</h1>
      {isLoading && <p>Загрузка...</p>}
      {isError && <p className="text-red-600">Нет прав (нужен organization:read) или нет сессии.</p>}
      {data && (
        <table className="w-full text-sm border">
          <thead>
            <tr className="bg-slate-100">
              <th className="border px-2 py-1">ID</th>
              <th className="border px-2 py-1">Название</th>
              <th className="border px-2 py-1">ИНН</th>
            </tr>
          </thead>
          <tbody>
            {data.map((o: any) => (
              <tr key={o.id}>
                <td className="border px-2 py-1">{o.id}</td>
                <td className="border px-2 py-1">{o.full_name}</td>
                <td className="border px-2 py-1">{o.inn}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <Link className="underline text-sm mt-4 inline-block" to="/me">← Кабинет</Link>
    </div>
  );
}
