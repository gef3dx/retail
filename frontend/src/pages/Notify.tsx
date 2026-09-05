import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

const TYPES = ['ORDER_CREATED', 'ORDER_STATUS_CHANGED', 'STOCK_LOW', 'STOCK_ARRIVED', 'RECEIPT_SOLD', 'SYSTEM_ALERT', 'PROMOTION'];
const CHANNELS = ['WEB', 'EMAIL', 'SMS', 'TELEGRAM', 'PUSH'];

export function Notify() {
  const qc = useQueryClient();
  const [msg, setMsg] = useState('');
  const [manual, setManual] = useState({ type: 'SYSTEM_ALERT', channel: 'WEB', text: '' });

  const inbox = useQuery({
    queryKey: ['inbox'],
    queryFn: async () => (await api.get('/notify/inbox')).data,
    refetchInterval: 5000,
  });
  const queue = useQuery({
    queryKey: ['nqueue'],
    queryFn: async () => (await api.get('/notify/queue')).data,
    refetchInterval: 5000,
  });
  const prefs = useQuery({ queryKey: ['prefs'], queryFn: async () => (await api.get('/notify/preferences')).data });
  const templates = useQuery({ queryKey: ['templates'], queryFn: async () => (await api.get('/notify/templates')).data });

  const viewed = useMutation({
    mutationFn: async (id: number) => (await api.post(`/notify/inbox/${id}/viewed`, {})).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['inbox'] }),
  });
  const setPref = useMutation({
    mutationFn: async (p: { type: string; channel: string; enabled: boolean }) =>
      (await api.put('/notify/preferences', p)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['prefs'] }),
  });
  const send = useMutation({
    mutationFn: async () =>
      (await api.post('/notify/send', {
        org_id: 1, type: manual.type, channels: [manual.channel],
        body: manual.text, data: { message: manual.text },
      })).data,
    onSuccess: () => { setMsg('Поставлено в очередь'); setManual({ ...manual, text: '' }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'send failed'),
  });

  const prefOf = (t: string, c: string) => prefs.data?.find((p: any) => p.type === t && p.channel === c);

  return (
    <div className="p-6 max-w-5xl mx-auto grid grid-cols-2 gap-4">
      <h1 className="text-xl font-bold col-span-2">
        Уведомления <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Входящие (WEB, live)</h2>
        {(inbox.data ?? []).map((n: any) => (
          <div key={n.id} className={`text-sm border-b py-1 ${n.status === 'VIEWED' ? 'text-slate-400' : ''}`}>
            <div className="flex justify-between">
              <b>{n.type}</b>
              {n.status !== 'VIEWED' && (
                <button className="underline text-xs" onClick={() => viewed.mutate(n.id)}>прочитано</button>
              )}
            </div>
            <p>{n.subject} {n.body}</p>
          </div>
        ))}
        {(inbox.data ?? []).length === 0 && <p className="text-sm text-slate-500">Пусто</p>}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Очередь</h2>
        {(queue.data ?? []).slice(0, 12).map((q: any) => (
          <div key={q.id} className="flex justify-between text-sm border-b py-1">
            <span>{q.type} → {q.channel}</span>
            <span className={q.status === 'FAILED' ? 'text-red-600' : 'text-amber-600'}>{q.status} ({q.attempts})</span>
          </div>
        ))}
        {(queue.data ?? []).length === 0 && <p className="text-sm text-slate-500">Очередь пуста</p>}
        <h2 className="font-bold mb-2 mt-4 text-sm">Отправить вручную</h2>
        <div className="flex gap-1 text-sm">
          <select className="border rounded px-1" value={manual.type} onChange={(e) => setManual({ ...manual, type: e.target.value })}>
            {TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
          <select className="border rounded px-1" value={manual.channel} onChange={(e) => setManual({ ...manual, channel: e.target.value })}>
            {CHANNELS.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
          <input className="border rounded px-2 py-1 flex-1" placeholder="Текст" value={manual.text} onChange={(e) => setManual({ ...manual, text: e.target.value })} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => send.mutate()}>→</button>
        </div>
        {msg && <p className="text-sm mt-1">{msg}</p>}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Мои предпочтения (клик — переключить)</h2>
        {TYPES.map((t) => (
          <div key={t} className="text-sm border-b py-1">
            <p className="font-mono text-xs">{t}</p>
            <div className="flex gap-2">
              {CHANNELS.map((c) => {
                const p = prefOf(t, c);
                const on = !p || p.enabled;
                return (
                  <button
                    key={c}
                    title={`${t}/${c}`}
                    className={`text-xs px-1 rounded ${on ? 'bg-green-100' : 'bg-slate-200 line-through'}`}
                    onClick={() => setPref.mutate({ type: t, channel: c, enabled: !on })}
                  >
                    {c}
                  </button>
                );
              })}
            </div>
          </div>
        ))}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Шаблоны</h2>
        {(templates.data ?? []).map((t: any) => (
          <div key={t.id} className="text-sm border-b py-1">
            <p className="font-mono text-xs">{t.type} → {t.channel}</p>
            <p className="text-slate-600">{t.preview}...</p>
          </div>
        ))}
      </div>
    </div>
  );
}
