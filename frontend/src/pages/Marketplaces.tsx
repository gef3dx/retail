import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

const badge: Record<string, string> = {
  ACTIVE: 'bg-green-100 text-green-800',
  INACTIVE: 'bg-slate-200 text-slate-600',
  DISABLED: 'bg-red-100 text-red-700',
};

export function Marketplaces() {
  const qc = useQueryClient();
  const [msg, setMsg] = useState('');
  const [wh, setWh] = useState('1');
  const [offer, setOffer] = useState({ provider: 'MARKET_OZON', product: '1', offer: '' });

  const providers = useQuery({
    queryKey: ['mktprov'],
    queryFn: async () => (await api.get('/market/providers', { params: { org_id: 1 } })).data,
  });
  const orders = useQuery({
    queryKey: ['mktorders'],
    queryFn: async () => (await api.get('/market/orders', { params: { org_id: 1 } })).data,
    refetchInterval: 8000,
  });
  const log = useQuery({
    queryKey: ['mktlog'],
    queryFn: async () => (await api.get('/market/sync-log', { params: { org_id: 1 } })).data,
    refetchInterval: 8000,
  });
  const offers = useQuery({
    queryKey: ['mktoffers'],
    queryFn: async () => (await api.get('/market/offers', { params: { org_id: 1 } })).data,
  });

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['mktorders'] });
    qc.invalidateQueries({ queryKey: ['mktlog'] });
  };
  const pull = useMutation({
    mutationFn: async (code: string) =>
      (await api.post(`/market/${code}/pull-orders`, { warehouse_id: Number(wh) }, { params: { org_id: 1 } })).data,
    onSuccess: (d) => { setMsg(`Заказов: ${d.orders}, создано: ${d.matched}, пропущено: ${d.skipped}`); refresh(); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'pull failed'),
  });
  const push = useMutation({
    mutationFn: async (code: string) =>
      (await api.post(`/market/${code}/push-stocks`, { warehouse_id: Number(wh) }, { params: { org_id: 1 } })).data,
    onSuccess: (d) => { setMsg(`Остатки выгружены: ${d.items}`); refresh(); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'push failed'),
  });
  const link = useMutation({
    mutationFn: async () =>
      (await api.post('/market/offers', {
        org_id: 1, provider_code: offer.provider,
        product_id: Number(offer.product), offer_id: offer.offer,
      })).data,
    onSuccess: () => { setOffer({ ...offer, offer: '' }); qc.invalidateQueries({ queryKey: ['mktoffers'] }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'link failed'),
  });

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <h1 className="text-xl font-bold mb-1">
        Маркетплейсы <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <p className="text-sm text-slate-500 mb-3">
        Ключи — в <Link className="underline" to="/integrations">интеграциях</Link>. Склад для синка:
        <input className="border rounded px-1 w-12 ml-1" value={wh} onChange={(e) => setWh(e.target.value)} />
      </p>
      {msg && <p className="text-sm mb-2">{msg}</p>}
      <div className="grid grid-cols-3 gap-2 mb-4">
        {(providers.data ?? []).map((p: any) => (
          <div key={p.code} className="border rounded p-3">
            <div className="flex items-center gap-2">
              <b className="text-sm">{p.name}</b>
              <span className={`text-xs px-2 rounded ${badge[p.status] ?? ''}`}>{p.status}</span>
            </div>
            {p.missing?.length > 0 && <p className="text-xs text-amber-700">Нужны ключи: {p.missing.join(', ')}</p>}
            <div className="flex gap-2 mt-2 text-sm">
              <button className="px-2 py-1 border rounded disabled:opacity-40" disabled={p.status !== 'ACTIVE'}
                onClick={() => pull.mutate(p.code)}>Заказы ↓</button>
              <button className="px-2 py-1 border rounded disabled:opacity-40" disabled={p.status !== 'ACTIVE'}
                onClick={() => push.mutate(p.code)}>Остатки ↑</button>
            </div>
          </div>
        ))}
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="border rounded p-3">
          <h2 className="font-bold text-sm mb-2">Заказы маркетов</h2>
          {(orders.data ?? []).map((o: any) => (
            <p key={o.id} className="text-sm border-b py-1">
              {o.provider_code} {o.external_order_id} — {o.status}
              {o.sales_order_id ? ` → заказ №${o.sales_order_id}` : ''} {o.error_message ?? ''}
            </p>
          ))}
          <h2 className="font-bold text-sm mt-3 mb-2">Связки товар ↔ оффер</h2>
          <div className="flex gap-1 text-sm mb-2">
            <select className="border rounded px-1" value={offer.provider} onChange={(e) => setOffer({ ...offer, provider: e.target.value })}>
              <option value="MARKET_OZON">Ozon</option>
              <option value="MARKET_WB">WB</option>
              <option value="MARKET_YANDEX">Yandex</option>
            </select>
            <input className="border rounded px-2 py-1 w-16" placeholder="Товар ID" value={offer.product} onChange={(e) => setOffer({ ...offer, product: e.target.value })} />
            <input className="border rounded px-2 py-1 flex-1" placeholder="Offer ID" value={offer.offer} onChange={(e) => setOffer({ ...offer, offer: e.target.value })} />
            <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => link.mutate()}>+</button>
          </div>
          {(offers.data ?? []).map((l: any) => (
            <p key={l.id} className="text-xs text-slate-600">{l.provider_code} {l.offer_id} → товар {l.product_id}</p>
          ))}
        </div>
        <div className="border rounded p-3">
          <h2 className="font-bold text-sm mb-2">Журнал синхронизаций</h2>
          {(log.data ?? []).map((l: any) => (
            <p key={l.id} className="text-sm border-b py-1">
              {l.operation} {l.direction} — <b>{l.status}</b> ({l.items_ok}/{l.items_total}) {l.error_message ?? ''}
            </p>
          ))}
        </div>
      </div>
    </div>
  );
}
