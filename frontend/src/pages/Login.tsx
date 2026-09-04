import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';

export function Login() {
  const { data, isError, refetch } = useQuery({
    queryKey: ['ping'],
    queryFn: async () => (await api.get('/ping')).data,
    retry: false,
  });

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-100">
      <div className="bg-white p-8 rounded shadow w-96">
        <h1 className="text-xl font-bold mb-2">RMS — вход (Этап 1)</h1>
        <p className="text-sm text-slate-500 mb-4">
          Скелет. Auth появится в Этапе 2. Проверка связи с backend:
        </p>
        <button
          onClick={() => refetch()}
          className="px-4 py-2 bg-slate-900 text-white rounded"
        >
          Проверить /api/v1/ping
        </button>
        <pre className="mt-4 text-xs bg-slate-50 p-2 rounded">
          {isError ? 'backend недоступен' : JSON.stringify(data ?? '...', null, 2)}
        </pre>
      </div>
    </div>
  );
}
