import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

function period() {
  const d = new Date();
  return { year: d.getFullYear(), quarter: Math.floor(d.getMonth() / 3) + 1 };
}

export function Reports() {
  const qc = useQueryClient();
  const p = period();
  const [year, setYear] = useState(String(p.year));
  const [quarter, setQuarter] = useState(String(p.quarter));
  const [msg, setMsg] = useState('');
  const q = { org_id: 1, year, quarter };

  const sales = useQuery({
    queryKey: ['salesbook', year, quarter],
    queryFn: async () => (await api.get('/tax/sales-book', { params: q })).data,
  });
  const purch = useQuery({
    queryKey: ['purchbook', year, quarter],
    queryFn: async () => (await api.get('/tax/purchase-book', { params: q })).data,
  });
  const decls = useQuery({
    queryKey: ['decls'],
    queryFn: async () => (await api.get('/tax/declarations', { params: { org_id: 1 } })).data,
  });
  const summary = useQuery({
    queryKey: ['summary'],
    queryFn: async () => (await api.get('/tax/summary', { params: { org_id: 1 } })).data,
  });

  const close = useMutation({
    mutationFn: async () =>
      (await api.post(`/tax/close?org_id=1`, { year: Number(year), quarter: Number(quarter), decl_type: 'NDS' })).data,
    onSuccess: (d) => {
      setMsg(`Закрыто: продажи ${d.total_sales}, НДС к уплате ${d.vat_due}`);
      qc.invalidateQueries({ queryKey: ['decls'] });
    },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'close failed'),
  });
  const submit = useMutation({
    mutationFn: async (id: number) => (await api.post(`/tax/declarations/${id}/submit?org_id=1`, {})).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['decls'] }),
  });

  async function download(book: string) {
    const r = await api.get(`/tax/export/${book}`, {
      params: { org_id: 1, year, quarter },
      responseType: 'blob',
    });
    const url = URL.createObjectURL(r.data);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${book}_${year}_q${quarter}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const maxDay = Math.max(1, ...(summary.data?.by_day ?? []).map((d: any) => d.total));

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <h1 className="text-xl font-bold mb-2">
        Отчёты и налоги <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <div className="flex gap-2 items-center text-sm mb-3">
        <input className="border rounded px-2 py-1 w-20" value={year} onChange={(e) => setYear(e.target.value)} />
        <select className="border rounded px-2 py-1" value={quarter} onChange={(e) => setQuarter(e.target.value)}>
          <option value="1">Q1</option><option value="2">Q2</option>
          <option value="3">Q3</option><option value="4">Q4</option>
        </select>
        <button className="underline text-sm" onClick={() => download('sales')}>
          CSV продаж
        </button>
        <button className="underline text-sm" onClick={() => download('purchase')}>
          CSV покупок
        </button>
        <button className="px-3 py-1 bg-slate-900 text-white rounded" onClick={() => close.mutate()}>
          Закрыть период (НДС)
        </button>
      </div>
      {msg && <p className="text-sm mb-2">{msg}</p>}

      <div className="grid grid-cols-3 gap-4">
        <div className="border rounded p-3">
          <h2 className="font-bold text-sm mb-2">Дашборд (30 дней)</h2>
          <p className="text-sm">Выручка: <b>{summary.data?.revenue_30d ?? 0}</b></p>
          <p className="text-sm">Чеков: <b>{summary.data?.receipts_30d ?? 0}</b></p>
          <p className="text-sm">НДС исходящий: <b>{summary.data?.vat_out_30d ?? 0}</b></p>
          <div className="mt-2">
            {(summary.data?.by_day ?? []).map((d: any) => (
              <div key={d.day} className="flex items-center gap-1 text-xs">
                <span className="w-20">{d.day.slice(5)}</span>
                <div className="bg-slate-800 h-3" style={{ width: `${Math.max(2, (d.total / maxDay) * 120)}px` }} />
                <span>{d.total}</span>
              </div>
            ))}
          </div>
          <h3 className="font-bold text-xs mt-2">Топ товаров</h3>
          {(summary.data?.top_products ?? []).map((t: any) => (
            <p key={t.sku} className="text-xs">{t.name} × {t.qty} = {t.total}</p>
          ))}
        </div>
        <div className="border rounded p-3">
          <h2 className="font-bold text-sm mb-2">Книга продаж</h2>
          {(sales.data ?? []).map((e: any) => (
            <p key={e.entry_number} className="text-xs border-b py-1">
              {e.entry_number}. {e.document_type} №{e.document_number} — {e.total_amount}
            </p>
          ))}
          <h2 className="font-bold text-sm mt-3 mb-2">Книга покупок</h2>
          {(purch.data ?? []).map((e: any) => (
            <p key={e.entry_number} className="text-xs border-b py-1">
              {e.entry_number}. {e.document_number} — {e.total_amount}
            </p>
          ))}
        </div>
        <div className="border rounded p-3">
          <h2 className="font-bold text-sm mb-2">Декларации</h2>
          {(decls.data ?? []).map((d: any) => (
            <div key={d.id} className="text-xs border-b py-1">
              <p>{d.year} Q{d.quarter} {d.decl_type} — <b>{d.status}</b></p>
              <p>Продажи {d.total_sales} / НДС {d.total_vat_out}; покупки {d.total_purchases} / НДС {d.total_vat_in}</p>
              <p>К уплате: <b>{d.vat_due}</b>{' '}
                {d.status === 'DRAFT' && (
                  <button className="underline" onClick={() => submit.mutate(d.id)}>сдать</button>
                )}
              </p>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
