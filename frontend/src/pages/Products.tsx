import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Products() {
  const qc = useQueryClient();
  const [q, setQ] = useState('');
  const [marked, setMarked] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [f, setF] = useState({ sku: '', gtin: '', name: '', base_price: '', vat_rate: '20', is_marked: false, retail_price: '' });
  const [err, setErr] = useState('');

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['products', q, marked],
    queryFn: async () =>
      (await api.get('/products', { params: { q: q || undefined, marked: marked || undefined, org_id: 1 } })).data,
  });

  const create = useMutation({
    mutationFn: async () => {
      const body: any = {
        sku: f.sku,
        name: f.name,
        org_id: 1,
        vat_rate: f.vat_rate ? Number(f.vat_rate) : undefined,
        is_marked: f.is_marked,
      };
      if (f.gtin) body.gtin = f.gtin;
      if (f.base_price) body.base_price = Number(f.base_price);
      if (f.retail_price) body.retail_price = Number(f.retail_price);
      return (await api.post('/products', body)).data;
    },
    onSuccess: () => {
      setShowForm(false);
      setF({ sku: '', gtin: '', name: '', base_price: '', vat_rate: '20', is_marked: false, retail_price: '' });
      qc.invalidateQueries({ queryKey: ['products'] });
    },
    onError: (e: any) => setErr(e.response?.data?.error ?? 'create failed'),
  });

  return (
    <div className="p-8 max-w-4xl mx-auto">
      <div className="flex items-center gap-3 mb-4">
        <h1 className="text-xl font-bold">Товары</h1>
        <input
          className="border rounded px-3 py-1 text-sm flex-1"
          placeholder="Поиск: название, SKU, GTIN..."
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <label className="text-sm flex items-center gap-1">
          <input type="checkbox" checked={marked} onChange={(e) => setMarked(e.target.checked)} />
          маркированные
        </label>
        <button className="px-3 py-1 border rounded text-sm" onClick={() => refetch()}>Найти</button>
        <button className="px-3 py-1 bg-slate-900 text-white rounded text-sm" onClick={() => setShowForm(!showForm)}>
          + Товар
        </button>
      </div>

      {showForm && (
        <div className="border rounded p-4 mb-4 grid grid-cols-3 gap-2 text-sm">
          <input className="border rounded px-2 py-1" placeholder="SKU*" value={f.sku} onChange={(e) => setF({ ...f, sku: e.target.value })} />
          <input className="border rounded px-2 py-1" placeholder="GTIN" value={f.gtin} onChange={(e) => setF({ ...f, gtin: e.target.value })} />
          <input className="border rounded px-2 py-1 col-span-1" placeholder="Название*" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
          <input className="border rounded px-2 py-1" placeholder="Себестоимость" value={f.base_price} onChange={(e) => setF({ ...f, base_price: e.target.value })} />
          <input className="border rounded px-2 py-1" placeholder="НДС" value={f.vat_rate} onChange={(e) => setF({ ...f, vat_rate: e.target.value })} />
          <input className="border rounded px-2 py-1" placeholder="Розница" value={f.retail_price} onChange={(e) => setF({ ...f, retail_price: e.target.value })} />
          <label className="flex items-center gap-1">
            <input type="checkbox" checked={f.is_marked} onChange={(e) => setF({ ...f, is_marked: e.target.checked })} />
            маркируемый
          </label>
          <button className="px-3 py-1 bg-slate-900 text-white rounded" onClick={() => { setErr(''); create.mutate(); }}>
            Создать
          </button>
          {err && <p className="text-red-600 col-span-3">{err}</p>}
        </div>
      )}

      {isLoading && <p>Загрузка...</p>}
      {isError && <p className="text-red-600">Нет прав (нужен product:read).</p>}
      {data && (
        <table className="w-full text-sm border">
          <thead>
            <tr className="bg-slate-100">
              <th className="border px-2 py-1">SKU</th>
              <th className="border px-2 py-1">Название</th>
              <th className="border px-2 py-1">GTIN</th>
              <th className="border px-2 py-1">НДС</th>
              <th className="border px-2 py-1">Марк.</th>
              <th className="border px-2 py-1">Розница</th>
            </tr>
          </thead>
          <tbody>
            {data.map((p: any) => (
              <tr key={p.id}>
                <td className="border px-2 py-1">{p.sku}</td>
                <td className="border px-2 py-1">{p.name}</td>
                <td className="border px-2 py-1">{p.gtin ?? '—'}</td>
                <td className="border px-2 py-1">{p.vat_rate}</td>
                <td className="border px-2 py-1">{p.is_marked ? 'да' : '—'}</td>
                <td className="border px-2 py-1">{p.retail_price ?? '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <Link className="underline text-sm mt-4 inline-block" to="/me">← Кабинет</Link>
    </div>
  );
}
