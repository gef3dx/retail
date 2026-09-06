import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

export function Egais() {
  const qc = useQueryClient();
  const [msg, setMsg] = useState('');
  const [doc, setDoc] = useState({ type: 'WayBill', xml: '' });

  const status = useQuery({
    queryKey: ['utmstatus'],
    queryFn: async () => (await api.get('/egais/status', { params: { org_id: 1 } })).data,
    retry: false,
    refetchInterval: 15000,
  });
  const docs = useQuery({
    queryKey: ['egaisdocs'],
    queryFn: async () => (await api.get('/egais/documents', { params: { org_id: 1 } })).data,
    refetchInterval: 8000,
  });

  const send = useMutation({
    mutationFn: async () =>
      (await api.post('/egais/documents', { org_id: 1, doc_type: doc.type, xml: doc.xml })).data,
    onSuccess: (d) => {
      setMsg(`Документ №${d.id}: ${d.status}`);
      setDoc({ ...doc, xml: '' });
      qc.invalidateQueries({ queryKey: ['egaisdocs'] });
    },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'send failed'),
  });

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <h1 className="text-xl font-bold mb-1">
        ЕГАИС <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <p className="text-sm text-slate-500 mb-3">
        URL УТМ — в <Link className="underline" to="/integrations">интеграциях</Link> (EGAIS_UTM).
      </p>
      {msg && <p className="text-sm mb-2">{msg}</p>}
      <div className="border rounded p-3 mb-3 text-sm">
        <b>УТМ:</b>{' '}
        {status.data?.reachable
          ? <span className="text-green-700">на связи{status.data.version ? ` (${status.data.version})` : ''}</span>
          : <span className="text-red-600">недоступен{status.error ? `: ${status.error}` : ''}</span>}
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="border rounded p-3">
          <h2 className="font-bold text-sm mb-2">Новый документ</h2>
          <input className="border rounded px-2 py-1 text-sm w-full mb-1" placeholder="Тип (WayBill, ActWriteOff...)"
            value={doc.type} onChange={(e) => setDoc({ ...doc, type: e.target.value })} />
          <textarea className="border rounded px-2 py-1 text-xs w-full h-28 font-mono" placeholder="<xml/>"
            value={doc.xml} onChange={(e) => setDoc({ ...doc, xml: e.target.value })} />
          <button className="px-3 py-1 bg-slate-900 text-white rounded text-sm mt-1" onClick={() => send.mutate()}>
            Отправить в УТМ
          </button>
        </div>
        <div className="border rounded p-3">
          <h2 className="font-bold text-sm mb-2">Документы</h2>
          {(docs.data ?? []).map((d: any) => (
            <div key={d.id} className="text-sm border-b py-1">
              <div className="flex justify-between">
                <span>№{d.id} {d.doc_type}</span>
                <b className={d.status === 'ACCEPTED' ? 'text-green-700' : d.status === 'FAILED' ? 'text-red-600' : ''}>
                  {d.status}
                </b>
              </div>
              {d.reply && <p className="text-xs text-slate-500">{d.reply}</p>}
              {d.error_message && <p className="text-xs text-red-600">{d.error_message}</p>}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
