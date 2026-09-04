import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Orders() {
  const qc = useQueryClient();
  const [f, setF] = useState({ buyer: '2', wh: '1', pid: '1', qty: '5' });
  const [sel, setSel] = useState<number | null>(null);
  const [msg, setMsg] = useState('');

  const orders = useQuery({ queryKey: ['orders'], queryFn: async () => (await api.get('/orders')).data, refetchInterval: 5000 });
  const detail = useQuery({
    queryKey: ['order', sel],
    queryFn: async () => (await api.get(`/orders/${sel}`)).data,
    enabled: sel != null,
  });

  const create = useMutation({
    mutationFn: async () =>
      (await api.post('/orders', {
        warehouse_id: Number(f.wh), buyer_id: Number(f.buyer),
        lines: [{ product_id: Number(f.pid), quantity: Number(f.qty) }],
      })).data,
    onSuccess: (d) => { setSel(d.id); qc.invalidateQueries({ queryKey: ['orders'] }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'create failed'),
  });
  const act = (id: number, a: string) =>
    api.post(`/orders/${id}/${a}`, {}).then(() => {
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['order', sel] });
    }).catch((e: any) => setMsg(e.response?.data?.error ?? `${a} failed`));
  const ship = (id: number) =>
    api.post('/shipments', { order_id: id }).then(() => {
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['order', sel] });
      setMsg('Отгружено');
    }).catch((e: any) => setMsg(e.response?.data?.error ?? 'ship failed'));

  return (
    <div className="p-6 max-w-5xl mx-auto grid grid-cols-2 gap-4">
      <h1 className="text-xl font-bold col-span-2">
        Заказы <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Новый заказ (1 строка, цена = розница)</h2>
        <div className="flex gap-1 text-sm">
          <input className="border rounded px-2 py-1 w-16" title="Покупатель ID" value={f.buyer} onChange={(e) => setF({ ...f, buyer: e.target.value })} />
          <input className="border rounded px-2 py-1 w-16" title="Склад ID" value={f.wh} onChange={(e) => setF({ ...f, wh: e.target.value })} />
          <input className="border rounded px-2 py-1 w-16" title="Товар ID" value={f.pid} onChange={(e) => setF({ ...f, pid: e.target.value })} />
          <input className="border rounded px-2 py-1 w-16" title="Кол-во" value={f.qty} onChange={(e) => setF({ ...f, qty: e.target.value })} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => create.mutate()}>+</button>
        </div>
        {msg && <p className="text-sm mt-2">{msg}</p>}
        <div className="mt-3">
          {(orders.data ?? []).map((o: any) => (
            <div key={o.id} className="flex justify-between text-sm border-b py-1">
              <button className="underline" onClick={() => setSel(o.id)}>{o.number} — {o.buyer}</button>
              <span>{o.total} / <b>{o.status}</b></span>
            </div>
          ))}
        </div>
      </div>
      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Детали {sel ?? '—'}</h2>
        {detail.data && (
          <>
            {(detail.data.lines ?? []).map((l: any) => (
              <p key={l.id} className="text-sm border-b py-1">
                {l.name} × {l.quantity} (резерв {l.reserved}, отгружено {l.shipped})
              </p>
            ))}
            <div className="flex gap-2 mt-3 text-sm">
              {detail.data.status === 'DRAFT' && (
                <button className="px-3 py-1 bg-slate-900 text-white rounded" onClick={() => act(sel!, 'confirm')}>Подтвердить</button>
              )}
              {(detail.data.status === 'CONFIRMED' || detail.data.status === 'SHIPPED') && (
                <button className="px-3 py-1 bg-green-700 text-white rounded" onClick={() => ship(sel!)}>Отгрузить остатки</button>
              )}
              {!['CANCELED', 'COMPLETED'].includes(detail.data.status) && (
                <button className="px-3 py-1 border rounded" onClick={() => act(sel!, 'cancel')}>Отменить</button>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
