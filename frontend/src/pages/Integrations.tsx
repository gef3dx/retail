import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

type P = {
  code: string; name: string; kind: string; status: string; enabled: boolean;
  emulator: boolean; keys: { key: string; name: string; secret: boolean; required: boolean }[];
  has_value: Record<string, boolean>; missing: string[];
};

const badge: Record<string, string> = {
  ACTIVE: 'bg-green-100 text-green-800',
  INACTIVE: 'bg-slate-200 text-slate-600',
  DISABLED: 'bg-red-100 text-red-700',
};

const kindName: Record<string, string> = {
  OFD: 'Кассы / ОФД', GISMT: 'Маркировка / ГИС МТ', NOTIFY: 'Уведомления',
  DELIVERY: 'Доставка', MARKET: 'Маркетплейсы', EGAIS: 'ЕГАИС',
};

export function Integrations() {
  const qc = useQueryClient();
  const [open, setOpen] = useState<string | null>(null);
  const [vals, setVals] = useState<Record<string, string>>({});
  const [msg, setMsg] = useState('');

  const { data } = useQuery({
    queryKey: ['integrations'],
    queryFn: async () => (await api.get('/integrations', { params: { org_id: 1 } })).data,
    refetchInterval: 8000,
  });

  const save = useMutation({
    mutationFn: async (p: { code: string; body: any }) =>
      (await api.put(`/integrations/${p.code}`, p.body, { params: { org_id: 1 } })).data,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['integrations'] }); setVals({}); setMsg('Сохранено'); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'save failed'),
  });
  const clear = useMutation({
    mutationFn: async (code: string) => (await api.delete(`/integrations/${code}`, { params: { org_id: 1 } })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['integrations'] }),
  });
  const test = useMutation({
    mutationFn: async (code: string) =>
      (await api.post(`/integrations/${code}/test`, {}, { params: { org_id: 1 } })).data,
    onSuccess: (d) => setMsg(d.ok ? `Тест OK: ${d.message}` : `Тест: ${d.message}`),
    onError: (e: any) => setMsg(e.response?.data?.message ?? e.response?.data?.error ?? 'test failed'),
  });

  const groups: Record<string, P[]> = {};
  ((data ?? []) as P[]).forEach((p) => {
    (groups[p.kind] ??= []).push(p);
  });

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <h1 className="text-xl font-bold mb-1">
        Интеграции <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <p className="text-sm text-slate-500 mb-4">
        Без ключей функции провайдера неактивны. Эмуляторы работают из коробки (только для разработки).
      </p>
      {msg && <p className="text-sm mb-2">{msg}</p>}
      {Object.entries(groups).map(([kind, list]) => (
        <div key={kind} className="mb-4">
          <h2 className="font-bold text-sm mb-2">{kindName[kind] ?? kind}</h2>
          {list.map((p) => (
            <div key={p.code} className="border rounded p-3 mb-2">
              <div className="flex items-center gap-2">
                <b className="text-sm">{p.name}</b>
                <span className={`text-xs px-2 rounded ${badge[p.status]}`}>{p.status}</span>
                {p.emulator && <span className="text-xs text-slate-400">эмулятор</span>}
                <span className="ml-auto flex gap-1">
                  <button className="text-xs underline" onClick={() => test.mutate(p.code)}>тест</button>
                  {!p.emulator && (
                    <button className="text-xs underline" onClick={() => setOpen(open === p.code ? null : p.code)}>
                      {open === p.code ? 'скрыть' : 'ключи'}
                    </button>
                  )}
                  <button
                    className="text-xs underline"
                    onClick={() => save.mutate({ code: p.code, body: { enabled: !p.enabled } })}
                  >
                    {p.enabled ? 'выключить' : 'включить'}
                  </button>
                </span>
              </div>
              {p.missing.length > 0 && (
                <p className="text-xs text-amber-700 mt-1">Не хватает ключей: {p.missing.join(', ')}</p>
              )}
              {open === p.code && (
                <div className="mt-2 grid grid-cols-2 gap-1">
                  {p.keys.map((k) => (
                    <label key={k.key} className="text-xs">
                      {k.name}{k.required ? ' *' : ''} {p.has_value[k.key] && <span className="text-green-700">✓ сохр.</span>}
                      <input
                        className="border rounded px-2 py-1 w-full text-sm"
                        type={k.secret ? 'password' : 'text'}
                        placeholder={p.has_value[k.key] ? '(сохранено, пусто = не менять)' : ''}
                        value={vals[p.code + ':' + k.key] ?? ''}
                        onChange={(e) => setVals({ ...vals, [p.code + ':' + k.key]: e.target.value })}
                      />
                    </label>
                  ))}
                  <div className="col-span-2 flex gap-2 mt-1">
                    <button
                      className="px-3 py-1 bg-slate-900 text-white rounded text-sm"
                      onClick={() => {
                        const creds: Record<string, string> = {};
                        p.keys.forEach((k) => {
                          const v = vals[p.code + ':' + k.key];
                          if (v) creds[k.key] = v;
                        });
                        save.mutate({ code: p.code, body: { credentials: creds } });
                      }}
                    >
                      Сохранить
                    </button>
                    <button className="px-3 py-1 border rounded text-sm" onClick={() => clear.mutate(p.code)}>
                      Очистить ключи
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
