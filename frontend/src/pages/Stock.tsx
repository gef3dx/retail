import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Stock() {
  const qc = useQueryClient();
  const [whId, setWhId] = useState('1');
  const [wh, setWh] = useState({ code: '', name: '' });
  const [cp, setCp] = useState({ inn: '', name: '', role: 'supplier' });
  const [rec, setRec] = useState({ supplier: '1', pid: '1', qty: '10', price: '30' });
  const [msg, setMsg] = useState('');

  const whs = useQuery({ queryKey: ['whs'], queryFn: async () => (await api.get('/warehouses', { params: { org_id: 1 } })).data });
  const bals = useQuery({
    queryKey: ['bals', whId],
    queryFn: async () => (await api.get('/stock/balances', { params: { warehouse_id: whId } })).data,
    enabled: !!whId,
  });
  const docs = useQuery({
    queryKey: ['rdocs', whId],
    queryFn: async () => (await api.get('/stock/receipts', { params: { warehouse_id: whId } })).data,
    enabled: !!whId,
  });

  const addWh = useMutation({
    mutationFn: async () => (await api.post('/warehouses', { org_id: 1, code: wh.code, name: wh.name })).data,
    onSuccess: () => { setWh({ code: '', name: '' }); qc.invalidateQueries({ queryKey: ['whs'] }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'warehouse failed'),
  });
  const addCp = useMutation({
    mutationFn: async () =>
      (await api.post('/counterparties', {
        org_id: 1, inn: cp.inn, full_name: cp.name,
        is_supplier: cp.role === 'supplier', is_buyer: cp.role === 'buyer',
      })).data,
    onSuccess: () => { setCp({ inn: '', name: '', role: 'supplier' }); setMsg('Контрагент создан'); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'counterparty failed'),
  });
  const addRec = useMutation({
    mutationFn: async () =>
      (await api.post('/stock/receipts', {
        warehouse_id: Number(whId), supplier_id: Number(rec.supplier),
        lines: [{ product_id: Number(rec.pid), quantity: Number(rec.qty), price: Number(rec.price) }],
      })).data,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['rdocs'] }); setMsg('Поступление создано (черновик)'); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'receipt failed'),
  });
  const postRec = useMutation({
    mutationFn: async (id: number) => (await api.post(`/stock/receipts/${id}/post`, {})).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rdocs'] });
      qc.invalidateQueries({ queryKey: ['bals'] });
      setMsg('Проведено');
    },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'post failed'),
  });

  return (
    <div className="p-6 max-w-5xl mx-auto grid grid-cols-2 gap-4">
      <h1 className="text-xl font-bold col-span-2">
        Склад <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Склады (org 1)</h2>
        <div className="flex gap-1 mb-2">
          <input className="border rounded px-2 py-1 text-sm w-20" placeholder="Код" value={wh.code} onChange={(e) => setWh({ ...wh, code: e.target.value })} />
          <input className="border rounded px-2 py-1 text-sm flex-1" placeholder="Название" value={wh.name} onChange={(e) => setWh({ ...wh, name: e.target.value })} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded text-sm" onClick={() => addWh.mutate()}>+</button>
        </div>
        {whs.data?.map((w: any) => (
          <p key={w.id} className="text-sm border-b py-1">
            <button className="underline" onClick={() => setWhId(String(w.id))}>{w.code}</button> — {w.name}
          </p>
        ))}
        <h2 className="font-bold mb-2 mt-4 text-sm">Контрагент</h2>
        <div className="flex gap-1">
          <input className="border rounded px-2 py-1 text-sm w-24" placeholder="ИНН" value={cp.inn} onChange={(e) => setCp({ ...cp, inn: e.target.value })} />
          <input className="border rounded px-2 py-1 text-sm flex-1" placeholder="Название" value={cp.name} onChange={(e) => setCp({ ...cp, name: e.target.value })} />
          <select className="border rounded px-1 text-sm" value={cp.role} onChange={(e) => setCp({ ...cp, role: e.target.value })}>
            <option value="supplier">Поставщик</option>
            <option value="buyer">Покупатель</option>
          </select>
          <button className="px-2 py-1 bg-slate-900 text-white rounded text-sm" onClick={() => addCp.mutate()}>+</button>
        </div>
        {msg && <p className="text-sm mt-2">{msg}</p>}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Остатки склада {whId}</h2>
        {(bals.data ?? []).map((b: any) => (
          <div key={b.product_id} className="flex justify-between text-sm border-b py-1">
            <span>{b.name}</span>
            <span>всего {b.quantity} / резерв {b.reserved} / <b>доступно {b.available}</b></span>
          </div>
        ))}
        <h2 className="font-bold mb-2 mt-4 text-sm">Поступление (1 строка)</h2>
        <div className="flex gap-1 text-sm">
          <input className="border rounded px-2 py-1 w-16" title="Поставщик ID" value={rec.supplier} onChange={(e) => setRec({ ...rec, supplier: e.target.value })} />
          <input className="border rounded px-2 py-1 w-16" title="Товар ID" value={rec.pid} onChange={(e) => setRec({ ...rec, pid: e.target.value })} />
          <input className="border rounded px-2 py-1 w-16" title="Кол-во" value={rec.qty} onChange={(e) => setRec({ ...rec, qty: e.target.value })} />
          <input className="border rounded px-2 py-1 w-16" title="Цена" value={rec.price} onChange={(e) => setRec({ ...rec, price: e.target.value })} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => addRec.mutate()}>+</button>
        </div>
        {(docs.data ?? []).map((d: any) => (
          <div key={d.id} className="flex justify-between text-sm border-b py-1">
            <span>{d.number} — {d.total}</span>
            {d.posted ? <span className="text-green-700">проведен</span> :
              <button className="underline" onClick={() => postRec.mutate(d.id)}>провести</button>}
          </div>
        ))}
      </div>
    </div>
  );
}
