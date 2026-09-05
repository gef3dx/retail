import { useEffect, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

const DOWS = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'];

export function Services() {
  const qc = useQueryClient();
  const [svc, setSvc] = useState({ sku: '', name: '', duration: '60', price: '' });
  const [res, setRes] = useState({ type: 'EMPLOYEE', name: '' });
  const [sched, setSched] = useState<{ [k: number]: { start: string; end: string; active: boolean } }>({});
  const [selRes, setSelRes] = useState<number | null>(null);
  const [msg, setMsg] = useState('');

  const services = useQuery({
    queryKey: ['services'],
    queryFn: async () => (await api.get('/products', { params: { type: 'SERVICE', org_id: 1, limit: 100 } })).data,
  });
  const resources = useQuery({
    queryKey: ['resources'],
    queryFn: async () => (await api.get('/resources', { params: { org_id: 1 } })).data,
  });
  const schedule = useQuery({
    queryKey: ['sched', selRes],
    queryFn: async () => (await api.get(`/resources/${selRes}/schedule`)).data,
    enabled: selRes != null,
  });
  useEffect(() => {
    if (schedule.data) {
      const m: typeof sched = {};
      schedule.data.forEach((x: any) => { m[x.dow] = { start: x.start, end: x.end, active: x.active }; });
      setSched(m);
    }
  }, [schedule.data]);

  const addSvc = useMutation({
    mutationFn: async () =>
      (await api.post('/products', {
        sku: svc.sku, name: svc.name, product_type: 'SERVICE',
        service_duration_minutes: Number(svc.duration),
        service_requires_booking: true, org_id: 1,
        base_price: svc.price ? Number(svc.price) : undefined,
        retail_price: svc.price ? Number(svc.price) : undefined,
      })).data,
    onSuccess: () => { setSvc({ sku: '', name: '', duration: '60', price: '' }); qc.invalidateQueries({ queryKey: ['services'] }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'service failed'),
  });
  const addRes = useMutation({
    mutationFn: async () => (await api.post('/resources', { org_id: 1, type: res.type, name: res.name })).data,
    onSuccess: () => { setRes({ type: 'EMPLOYEE', name: '' }); qc.invalidateQueries({ queryKey: ['resources'] }); },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'resource failed'),
  });
  const saveSched = useMutation({
    mutationFn: async () =>
      (await api.put(`/resources/${selRes}/schedule`, {
        days: [1, 2, 3, 4, 5, 6, 7]
          .filter((d) => sched[d])
          .map((d) => ({ dow: d, start: sched[d].start, end: sched[d].end, active: sched[d].active })),
      })).data,
    onSuccess: () => setMsg('Расписание сохранено'),
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'schedule failed'),
  });

  return (
    <div className="p-6 max-w-5xl mx-auto grid grid-cols-2 gap-4">
      <h1 className="text-xl font-bold col-span-2">
        Услуги и ресурсы <Link className="underline text-sm font-normal ml-2" to="/me">Кабинет</Link>
      </h1>
      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Услуги</h2>
        <div className="flex gap-1 mb-2 text-sm">
          <input className="border rounded px-2 py-1 w-20" placeholder="SKU" value={svc.sku} onChange={(e) => setSvc({ ...svc, sku: e.target.value })} />
          <input className="border rounded px-2 py-1 flex-1" placeholder="Название" value={svc.name} onChange={(e) => setSvc({ ...svc, name: e.target.value })} />
          <input className="border rounded px-2 py-1 w-14" title="Минут" value={svc.duration} onChange={(e) => setSvc({ ...svc, duration: e.target.value })} />
          <input className="border rounded px-2 py-1 w-20" title="Цена" value={svc.price} onChange={(e) => setSvc({ ...svc, price: e.target.value })} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => addSvc.mutate()}>+</button>
        </div>
        {(services.data ?? []).map((s: any) => (
          <p key={s.id} className="text-sm border-b py-1">{s.sku} — {s.name} ({s.service_duration_minutes} мин)</p>
        ))}
        {msg && <p className="text-sm mt-2">{msg}</p>}
      </div>
      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Ресурсы</h2>
        <div className="flex gap-1 mb-2 text-sm">
          <select className="border rounded px-1" value={res.type} onChange={(e) => setRes({ ...res, type: e.target.value })}>
            <option value="EMPLOYEE">Сотрудник</option>
            <option value="ROOM">Помещение</option>
            <option value="EQUIPMENT">Оборудование</option>
          </select>
          <input className="border rounded px-2 py-1 flex-1" placeholder="Название" value={res.name} onChange={(e) => setRes({ ...res, name: e.target.value })} />
          <button className="px-2 py-1 bg-slate-900 text-white rounded" onClick={() => addRes.mutate()}>+</button>
        </div>
        {(resources.data ?? []).map((r: any) => (
          <p key={r.id} className="text-sm border-b py-1">
            <button className="underline" onClick={() => setSelRes(r.id)}>{r.name}</button> ({r.type})
          </p>
        ))}
        {selRes != null && (
          <div className="mt-3">
            <h3 className="font-bold text-sm mb-1">Расписание ресурса {selRes}</h3>
            {schedule.data !== undefined && [1, 2, 3, 4, 5, 6, 7].map((d) => (
              <div key={d} className="flex gap-1 items-center text-sm mb-1">
                <span className="w-8">{DOWS[d - 1]}</span>
                <input className="border rounded px-1 w-16" type="time" value={sched[d]?.start ?? '09:00'}
                  onChange={(e) => setSched({ ...sched, [d]: { start: e.target.value, end: sched[d]?.end ?? '18:00', active: sched[d]?.active ?? true } })} />
                <input className="border rounded px-1 w-16" type="time" value={sched[d]?.end ?? '18:00'}
                  onChange={(e) => setSched({ ...sched, [d]: { start: sched[d]?.start ?? '09:00', end: e.target.value, active: sched[d]?.active ?? true } })} />
                <input type="checkbox" checked={sched[d]?.active ?? false}
                  onChange={(e) => setSched({ ...sched, [d]: { start: sched[d]?.start ?? '09:00', end: sched[d]?.end ?? '18:00', active: e.target.checked } })} />
              </div>
            ))}
            <button className="px-3 py-1 bg-slate-900 text-white rounded text-sm mt-1" onClick={() => saveSched.mutate()}>
              Сохранить расписание
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
