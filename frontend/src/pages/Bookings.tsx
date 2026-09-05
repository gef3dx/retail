import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

function today() {
  return new Date().toISOString().slice(0, 10);
}

export function Bookings() {
  const qc = useQueryClient();
  const [date, setDate] = useState(today());
  const [sel, setSel] = useState<number | null>(null);
  const [f, setF] = useState({ product: '1', resource: '1', start: '', name: '', phone: '' });
  const [msg, setMsg] = useState('');

  const list = useQuery({
    queryKey: ['bookings', date],
    queryFn: async () => (await api.get('/bookings', { params: { org_id: 1, date } })).data,
  });
  const detail = useQuery({
    queryKey: ['booking', sel],
    queryFn: async () => (await api.get(`/bookings/${sel}`)).data,
    enabled: sel != null,
  });
  const services = useQuery({
    queryKey: ['services'],
    queryFn: async () => (await api.get('/products', { params: { type: 'SERVICE', org_id: 1, limit: 100 } })).data,
  });
  const resources = useQuery({
    queryKey: ['resources'],
    queryFn: async () => (await api.get('/resources', { params: { org_id: 1 } })).data,
  });

  const create = useMutation({
    mutationFn: async () => {
      const d = new Date(f.start);
      return (
        await api.post('/bookings', {
          org_id: 1, product_id: Number(f.product), resource_ids: [Number(f.resource)],
          start: d.toISOString(), customer_name: f.name, customer_phone: f.phone,
        })
      ).data;
    },
    onSuccess: (d) => { setSel(d.id); qc.invalidateQueries({ queryKey: ['bookings'] }); setMsg(''); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'create failed'),
  });
  const setStatus = useMutation({
    mutationFn: async (s: string) => (await api.post(`/bookings/${sel}/status`, { status: s })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['bookings'] });
      qc.invalidateQueries({ queryKey: ['booking', sel] });
    },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'status failed'),
  });

  return (
    <div className="p-6 max-w-5xl mx-auto grid grid-cols-2 gap-4">
      <h1 className="text-xl font-bold col-span-2">
        Бронирования <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <div className="border rounded p-4">
        <div className="flex gap-2 mb-2 items-center">
          <h2 className="font-bold text-sm">Журнал</h2>
          <input className="border rounded px-2 py-1 text-sm" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </div>
        {(list.data ?? []).map((b: any) => (
          <div key={b.id} className="flex justify-between text-sm border-b py-1">
            <button className="underline" onClick={() => setSel(b.id)}>
              {b.start.slice(11, 16)} {b.customer || '—'} ({b.service})
            </button>
            <b>{b.status}</b>
          </div>
        ))}
        {(list.data ?? []).length === 0 && <p className="text-sm text-slate-500">Нет броней на дату</p>}
        <h2 className="font-bold text-sm mt-4 mb-2">Новая бронь</h2>
        <div className="grid grid-cols-2 gap-1 text-sm">
          <select className="border rounded px-2 py-1" value={f.product} onChange={(e) => setF({ ...f, product: e.target.value })}>
            {(services.data ?? []).map((s: any) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
          <select className="border rounded px-2 py-1" value={f.resource} onChange={(e) => setF({ ...f, resource: e.target.value })}>
            {(resources.data ?? []).map((r: any) => <option key={r.id} value={r.id}>{r.name}</option>)}
          </select>
          <input className="border rounded px-2 py-1" type="datetime-local" value={f.start} onChange={(e) => setF({ ...f, start: e.target.value })} />
          <input className="border rounded px-2 py-1" placeholder="Клиент" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
          <input className="border rounded px-2 py-1 col-span-2" placeholder="Телефон" value={f.phone} onChange={(e) => setF({ ...f, phone: e.target.value })} />
        </div>
        <button className="px-3 py-1 bg-slate-900 text-white rounded text-sm mt-2" onClick={() => create.mutate()}>
          Забронировать
        </button>
        {msg && <p className="text-sm mt-1 text-red-600">{msg}</p>}
      </div>
      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Детали {sel ?? '—'}</h2>
        {detail.data && (
          <>
            <p className="text-sm">Клиент: {detail.data.customer} {detail.data.phone}</p>
            <p className="text-sm">Время: {detail.data.start} — {detail.data.end}</p>
            <p className="text-sm">Статус: <b>{detail.data.status}</b></p>
            <div className="flex gap-2 mt-3 text-sm flex-wrap">
              {detail.data.status === 'PENDING' && (
                <>
                  <button className="px-3 py-1 bg-green-700 text-white rounded" onClick={() => setStatus.mutate('CONFIRMED')}>Подтвердить</button>
                  <button className="px-3 py-1 border rounded" onClick={() => setStatus.mutate('CANCELED')}>Отменить</button>
                </>
              )}
              {detail.data.status === 'CONFIRMED' && (
                <>
                  <button className="px-3 py-1 bg-slate-900 text-white rounded" onClick={() => setStatus.mutate('IN_PROGRESS')}>Начать</button>
                  <button className="px-3 py-1 border rounded" onClick={() => setStatus.mutate('CANCELED')}>Отменить</button>
                  <button className="px-3 py-1 border rounded" onClick={() => setStatus.mutate('NO_SHOW')}>Неявка</button>
                </>
              )}
              {detail.data.status === 'IN_PROGRESS' && (
                <button className="px-3 py-1 bg-green-700 text-white rounded" onClick={() => setStatus.mutate('COMPLETED')}>Завершить</button>
              )}
            </div>
            <h3 className="font-bold text-sm mt-3">История</h3>
            {(detail.data.history ?? []).map((h: any, i: number) => (
              <p key={i} className="text-xs text-slate-600">{h.at} — {h.status} {h.comment ?? ''}</p>
            ))}
          </>
        )}
      </div>
    </div>
  );
}
