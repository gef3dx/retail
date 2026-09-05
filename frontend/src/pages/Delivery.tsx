import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

const NEXT: Record<string, string[]> = {
  NEW: ['ASSIGNED', 'CANCELED'],
  ASSIGNED: ['PICKED_UP', 'CANCELED'],
  PICKED_UP: ['IN_TRANSIT', 'CANCELED'],
  IN_TRANSIT: ['ARRIVED', 'DELIVERED', 'RETURNED'],
  ARRIVED: ['DELIVERED', 'RETURNED'],
};

export function Delivery() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<'orders' | 'couriers' | 'zones'>('orders');
  const [sel, setSel] = useState<number | null>(null);
  const [msg, setMsg] = useState('');
  const [f, setF] = useState({ type: 'COURIER', address: '', name: '', phone: '' });
  const [assignId, setAssignId] = useState('');
  const [cour, setCour] = useState({ first: '', last: '', phone: '' });
  const [zone, setZone] = useState({ name: '', base: '' });

  const list = useQuery({
    queryKey: ['deliveries'],
    queryFn: async () => (await api.get('/deliveries', { params: { org_id: 1 } })).data,
    refetchInterval: 5000,
  });
  const detail = useQuery({
    queryKey: ['delivery', sel],
    queryFn: async () => (await api.get(`/deliveries/${sel}`)).data,
    enabled: sel != null,
  });
  const couriers = useQuery({
    queryKey: ['couriers'],
    queryFn: async () => (await api.get('/delivery/couriers', { params: { org_id: 1 } })).data,
  });
  const zones = useQuery({
    queryKey: ['zones'],
    queryFn: async () => (await api.get('/delivery/zones', { params: { org_id: 1 } })).data,
  });

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['deliveries'] });
    qc.invalidateQueries({ queryKey: ['delivery', sel] });
  };
  const create = useMutation({
    mutationFn: async () =>
      (await api.post('/deliveries', {
        org_id: 1, delivery_type: f.type, address: f.address,
        recipient_name: f.name, recipient_phone: f.phone,
      })).data,
    onSuccess: (d) => { setSel(d.id); refresh(); setMsg(''); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'create failed'),
  });
  const assign = useMutation({
    mutationFn: async () => (await api.post(`/deliveries/${sel}/assign`, { courier_id: Number(assignId) })).data,
    onSuccess: refresh,
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'assign failed'),
  });
  const accept = useMutation({
    mutationFn: async (v: boolean) => (await api.post(`/deliveries/${sel}/accept`, { accept: v })).data,
    onSuccess: refresh,
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'accept failed'),
  });
  const setStatus = useMutation({
    mutationFn: async (s: string) => (await api.post(`/deliveries/${sel}/status`, { status: s })).data,
    onSuccess: refresh,
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'status failed'),
  });
  const addCourier = useMutation({
    mutationFn: async () =>
      (await api.post('/delivery/couriers', {
        org_id: 1, first_name: cour.first, last_name: cour.last, phone: cour.phone,
      })).data,
    onSuccess: () => { setCour({ first: '', last: '', phone: '' }); qc.invalidateQueries({ queryKey: ['couriers'] }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'courier failed'),
  });
  const addZone = useMutation({
    mutationFn: async () =>
      (await api.post('/delivery/zones', { org_id: 1, name: zone.name, base_price: Number(zone.base) })).data,
    onSuccess: () => { setZone({ name: '', base: '' }); qc.invalidateQueries({ queryKey: ['zones'] }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'zone failed'),
  });

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <h1 className="text-xl font-bold mb-2">
        Доставка <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <div className="flex gap-2 mb-3 text-sm">
        {(['orders', 'couriers', 'zones'] as const).map((t) => (
          <button key={t} className={`px-3 py-1 border rounded ${tab === t ? 'bg-slate-900 text-white' : ''}`}
            onClick={() => setTab(t)}>
            {t === 'orders' ? 'Диспетчерская' : t === 'couriers' ? 'Курьеры' : 'Зоны'}
          </button>
        ))}
      </div>
      {msg && <p className="text-sm text-red-600 mb-2">{msg}</p>}

      {tab === 'orders' && (
        <div className="grid grid-cols-2 gap-4">
          <div className="border rounded p-4">
            <h2 className="font-bold text-sm mb-2">Заказы</h2>
            {(list.data ?? []).map((d: any) => (
              <div key={d.id} className="flex justify-between text-sm border-b py-1">
                <button className="underline" onClick={() => setSel(d.id)}>
                  №{d.id} {d.delivery_type} — {d.address.slice(0, 25)}
                </button>
                <b>{d.status}</b>
              </div>
            ))}
            <h2 className="font-bold text-sm mt-4 mb-2">Новая доставка</h2>
            <div className="grid grid-cols-2 gap-1 text-sm">
              <select className="border rounded px-2 py-1" value={f.type} onChange={(e) => setF({ ...f, type: e.target.value })}>
                <option value="COURIER">Курьер</option>
                <option value="PICKUP">Самовывоз</option>
                <option value="CDEK">СДЭК</option>
                <option value="POST">Почта</option>
              </select>
              <input className="border rounded px-2 py-1" placeholder="Адрес" value={f.address} onChange={(e) => setF({ ...f, address: e.target.value })} />
              <input className="border rounded px-2 py-1" placeholder="Получатель" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
              <input className="border rounded px-2 py-1" placeholder="Телефон" value={f.phone} onChange={(e) => setF({ ...f, phone: e.target.value })} />
            </div>
            <button className="px-3 py-1 bg-slate-900 text-white rounded text-sm mt-2" onClick={() => create.mutate()}>
              Создать
            </button>
          </div>
          <div className="border rounded p-4">
            <h2 className="font-bold mb-2 text-sm">Детали {sel ?? '—'}</h2>
            {detail.data && (
              <>
                <p className="text-sm">Адрес: {detail.data.address}</p>
                <p className="text-sm">Получатель: {detail.data.recipient} {detail.data.phone}</p>
                <p className="text-sm">Статус: <b>{detail.data.status}</b> {detail.data.tracking_number && `(трек ${detail.data.tracking_number})`}</p>
                <p className="text-sm">Назначения: {(detail.data.assignments ?? []).map((a: any) => `${a.courier} (${a.status})`).join(', ') || '—'}</p>
                {detail.data.status === 'NEW' && (
                  <div className="flex gap-1 mt-2 text-sm">
                    <select className="border rounded px-2 py-1" value={assignId} onChange={(e) => setAssignId(e.target.value)}>
                      <option value="">— курьер —</option>
                      {(couriers.data ?? []).map((c: any) => <option key={c.id} value={c.id}>{c.first_name} {c.last_name}</option>)}
                    </select>
                    <button className="px-3 py-1 bg-slate-900 text-white rounded" onClick={() => assign.mutate()}>
                      Назначить
                    </button>
                  </div>
                )}
                {(detail.data.assignments ?? []).some((a: any) => a.status === 'ASSIGNED') && (
                  <div className="flex gap-2 mt-2 text-sm">
                    <button className="px-3 py-1 bg-green-700 text-white rounded" onClick={() => accept.mutate(true)}>Принял</button>
                    <button className="px-3 py-1 border rounded" onClick={() => accept.mutate(false)}>Отклонить</button>
                  </div>
                )}
                <div className="flex gap-2 mt-2 text-sm flex-wrap">
                  {(NEXT[detail.data.status] ?? []).map((s) => (
                    <button key={s} className="px-3 py-1 border rounded" onClick={() => setStatus.mutate(s)}>{s}</button>
                  ))}
                </div>
                <h3 className="font-bold text-sm mt-3">История</h3>
                {(detail.data.history ?? []).map((h: any, i: number) => (
                  <p key={i} className="text-xs text-slate-600">{h.at} — {h.status} {h.comment ?? ''}</p>
                ))}
              </>
            )}
          </div>
        </div>
      )}

      {tab === 'couriers' && (
        <div className="border rounded p-4 max-w-xl">
          <div className="flex gap-1 text-sm mb-3">
            <input className="border rounded px-2 py-1" placeholder="Имя" value={cour.first} onChange={(e) => setCour({ ...cour, first: e.target.value })} />
            <input className="border rounded px-2 py-1" placeholder="Фамилия" value={cour.last} onChange={(e) => setCour({ ...cour, last: e.target.value })} />
            <input className="border rounded px-2 py-1" placeholder="Телефон" value={cour.phone} onChange={(e) => setCour({ ...cour, phone: e.target.value })} />
            <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => addCourier.mutate()}>+</button>
          </div>
          {(couriers.data ?? []).map((c: any) => (
            <p key={c.id} className="text-sm border-b py-1">
              {c.first_name} {c.last_name} — {c.phone} {c.is_available ? '' : '(занят)'}
            </p>
          ))}
        </div>
      )}

      {tab === 'zones' && (
        <div className="border rounded p-4 max-w-xl">
          <div className="flex gap-1 text-sm mb-3">
            <input className="border rounded px-2 py-1 flex-1" placeholder="Название" value={zone.name} onChange={(e) => setZone({ ...zone, name: e.target.value })} />
            <input className="border rounded px-2 py-1 w-28" placeholder="Тариф" value={zone.base} onChange={(e) => setZone({ ...zone, base: e.target.value })} />
            <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => addZone.mutate()}>+</button>
          </div>
          {(zones.data ?? []).map((z: any) => (
            <p key={z.id} className="text-sm border-b py-1">{z.name} — {z.base_price} руб.</p>
          ))}
        </div>
      )}
    </div>
  );
}
