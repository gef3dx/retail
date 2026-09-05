import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Marking() {
  const qc = useQueryClient();
  const [prodId, setProdId] = useState('1');
  const [batch, setBatch] = useState('');
  const [codes, setCodes] = useState('');
  const [check, setCheck] = useState('');
  const [checkRes, setCheckRes] = useState<any>(null);
  const [msg, setMsg] = useState('');

  const list = useQuery({
    queryKey: ['mcodes', prodId],
    queryFn: async () => (await api.get('/marking/codes', { params: { org_id: 1, product_id: prodId || undefined } })).data,
  });
  const queue = useQuery({
    queryKey: ['mqueue'],
    queryFn: async () => (await api.get('/marking/queue', { params: { org_id: 1 } })).data,
    refetchInterval: 4000,
  });
  const prov = useQuery({
    queryKey: ['gismtprov'],
    queryFn: async () => (await api.get('/gismt-active', { params: { org_id: 1 } })).data,
    retry: false,
  });
  const settings = useQuery({
    queryKey: ['gismtset'],
    queryFn: async () => (await api.get('/gismt-settings', { params: { org_id: 1 } })).data,
    retry: false,
  });
  const strict = useMutation({
    mutationFn: async (v: boolean) =>
      (await api.patch('/gismt-settings', { strict_online: v }, { params: { org_id: 1 } })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['gismtset'] }),
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'settings failed'),
  });

  const register = useMutation({
    mutationFn: async () =>
      (
        await api.post('/marking/codes', {
          org_id: 1,
          product_id: Number(prodId),
          batch_number: batch || undefined,
          codes: codes.split(/[\s,]+/).filter(Boolean),
        })
      ).data,
    onSuccess: (d) => {
      setMsg(`Зарегистрировано: ${d.registered}, дубли: ${d.duplicates}, отклонено: ${(d.rejected ?? []).length}`);
      setCodes('');
      qc.invalidateQueries({ queryKey: ['mcodes'] });
    },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'register failed'),
  });

  async function doCheck() {
    setCheckRes(null);
    try {
      const { data } = await api.get(`/marking/check/${encodeURIComponent(check.trim())}`);
      setCheckRes(data);
    } catch {
      setCheckRes({ error: 'код неизвестен' });
    }
  }

  return (
    <div className="p-6 max-w-5xl mx-auto grid grid-cols-2 gap-4">
      <h1 className="text-xl font-bold col-span-2">
        Маркировка «Честный знак» <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <div className="col-span-2 flex items-center gap-3 text-sm border rounded p-2">
        <span>ГИС МТ: <b>{prov.data?.code === 'GISMT_TRUEAPI' ? 'True API' : prov.data?.code ? 'эмулятор' : '—'}</b></span>
        <label className="flex items-center gap-1" title="Блокировать продажу маркировки без настроенного True API">
          <input type="checkbox" checked={!!settings.data?.strict_online}
            onChange={(e) => strict.mutate(e.target.checked)} />
          строгий онлайн-режим
        </label>
        <Link className="underline text-xs ml-auto" to="/integrations">ключи →</Link>
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Регистрация кодов</h2>
        <div className="flex gap-1 mb-2">
          <input className="border rounded px-2 py-1 text-sm w-20" placeholder="Prod ID" value={prodId} onChange={(e) => setProdId(e.target.value)} />
          <input className="border rounded px-2 py-1 text-sm flex-1" placeholder="Пачка (необязательно)" value={batch} onChange={(e) => setBatch(e.target.value)} />
        </div>
        <textarea
          className="border rounded px-2 py-1 text-sm w-full h-24"
          placeholder="Коды через пробел/запятую/перенос строки"
          value={codes}
          onChange={(e) => setCodes(e.target.value)}
        />
        <button className="px-4 py-1 bg-slate-900 text-white rounded text-sm mt-2" onClick={() => register.mutate()}>
          Зарегистрировать
        </button>
        {msg && <p className="text-sm mt-2">{msg}</p>}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Проверка кода</h2>
        <div className="flex gap-1">
          <input className="border rounded px-2 py-1 text-sm flex-1" placeholder="DataMatrix" value={check} onChange={(e) => setCheck(e.target.value)} />
          <button className="px-3 py-1 border rounded text-sm" onClick={doCheck}>Проверить</button>
        </div>
        {checkRes && (
          <pre className="text-xs bg-slate-50 border rounded p-2 mt-2">{JSON.stringify(checkRes, null, 2)}</pre>
        )}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Коды товара {prodId}</h2>
        {(list.data ?? []).map((c: any) => (
          <div key={c.id} className="flex justify-between text-sm border-b py-1">
            <span>...{c.code.slice(-8)}</span>
            <span className={c.status === 'AVAILABLE' ? 'text-green-700' : 'text-slate-500'}>{c.status}</span>
          </div>
        ))}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Очередь ГИС МТ</h2>
        {(queue.data ?? []).slice(0, 15).map((q: any) => (
          <div key={q.id} className="flex justify-between text-sm border-b py-1">
            <span>{q.operation} ...{q.code.slice(-5)}</span>
            <span className={q.status === 'COMPLETED' ? 'text-green-700' : 'text-amber-600'}>{q.status}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
