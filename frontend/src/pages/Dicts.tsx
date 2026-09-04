import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Dicts() {
  const qc = useQueryClient();
  const [cat, setCat] = useState({ code: '', name: '' });
  const [brand, setBrand] = useState('');

  const cats = useQuery({ queryKey: ['cats'], queryFn: async () => (await api.get('/categories')).data });
  const brands = useQuery({ queryKey: ['brands'], queryFn: async () => (await api.get('/brands')).data });

  const addCat = useMutation({
    mutationFn: async () => (await api.post('/categories', cat)).data,
    onSuccess: () => {
      setCat({ code: '', name: '' });
      qc.invalidateQueries({ queryKey: ['cats'] });
    },
  });
  const addBrand = useMutation({
    mutationFn: async () => (await api.post('/brands', { name: brand })).data,
    onSuccess: () => {
      setBrand('');
      qc.invalidateQueries({ queryKey: ['brands'] });
    },
  });

  return (
    <div className="p-8 max-w-3xl mx-auto grid grid-cols-2 gap-6">
      <div>
        <h2 className="font-bold mb-2">Категории</h2>
        <div className="flex gap-1 mb-2">
          <input className="border rounded px-2 py-1 text-sm w-20" placeholder="Код" value={cat.code} onChange={(e) => setCat({ ...cat, code: e.target.value })} />
          <input className="border rounded px-2 py-1 text-sm flex-1" placeholder="Название" value={cat.name} onChange={(e) => setCat({ ...cat, name: e.target.value })} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded text-sm" onClick={() => addCat.mutate()}>+</button>
        </div>
        {cats.data?.map((c: any) => (
          <p key={c.id} className="text-sm border-b py-1">{c.code} — {c.name}</p>
        ))}
      </div>
      <div>
        <h2 className="font-bold mb-2">Бренды</h2>
        <div className="flex gap-1 mb-2">
          <input className="border rounded px-2 py-1 text-sm flex-1" placeholder="Название" value={brand} onChange={(e) => setBrand(e.target.value)} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded text-sm" onClick={() => addBrand.mutate()}>+</button>
        </div>
        {brands.data?.map((b: any) => (
          <p key={b.id} className="text-sm border-b py-1">{b.name}</p>
        ))}
      </div>
      <Link className="underline text-sm col-span-2" to="/me">← Кабинет</Link>
    </div>
  );
}
