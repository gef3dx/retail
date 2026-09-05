import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../api/client';

type CartLine = { product_id: number; name: string; price: number; qty: number; is_marked: boolean; codes: string };

export function Cashier() {
  const qc = useQueryClient();
  const [regId, setRegId] = useState('1');
  const [code, setCode] = useState('');
  const [cart, setCart] = useState<CartLine[]>([]);
  const [payType, setPayType] = useState('CASH');
  const [cash, setCash] = useState('');
  const [msg, setMsg] = useState('');

  const regs = useQuery({ queryKey: ['regs'], queryFn: async () => (await api.get('/registers', { params: { org_id: 1 } })).data });
  const providers = useQuery({
    queryKey: ['ofdprov'],
    queryFn: async () => (await api.get('/ofd-active', { params: { org_id: 1 } })).data,
    retry: false,
  });
  const ofdActive = providers.data?.code ? providers.data : null;
  const shift = useQuery({
    queryKey: ['shift', regId],
    queryFn: async () => (await api.get('/shifts/open', { params: { register_id: regId } })).data,
    retry: false,
  });
  const receipts = useQuery({
    queryKey: ['creceipts', shift.data?.id],
    queryFn: async () => (await api.get('/receipts', { params: { shift_id: shift.data.id } })).data,
    enabled: !!shift.data?.id,
    refetchInterval: 3000,
  });

  const total = cart.reduce((s, l) => s + l.price * l.qty, 0);

  async function addByCode() {
    setMsg('');
    try {
      const { data: p } = await api.get(`/products/by-code/${encodeURIComponent(code)}`, { params: { org_id: 1 } });
      const price = p.retail_price ?? p.base_price;
      if (price == null) {
        setMsg('У товара нет цены');
        return;
      }
      setCart((c) => {
        const ex = c.find((l) => l.product_id === p.id);
        if (ex) return c.map((l) => (l.product_id === p.id ? { ...l, qty: l.qty + 1 } : l));
        return [...c, { product_id: p.id, name: p.name, price, qty: 1, is_marked: !!p.is_marked, codes: '' }];
      });
      setCode('');
    } catch {
      setMsg('Товар не найден');
    }
  }

  const sell = useMutation({
    mutationFn: async () => {
      const paid = payType === 'CARD' ? { payment_card: total } : { payment_cash: Number(cash) || total };
      return (
        await api.post('/receipts/sell', {
          cash_register_id: Number(regId),
          items: cart.map((l) => ({
            product_id: l.product_id,
            quantity: l.qty,
            marking_codes: l.is_marked ? l.codes.split(/[\s,]+/).filter(Boolean) : undefined,
          })),
          payment_type: payType,
          payment_cash: 0,
          payment_card: 0,
          ...paid,
        })
      ).data;
    },
    onSuccess: (d) => {
      setMsg(`Чек №${d.receipt_number} пробит`);
      setCart([]);
      setCash('');
      qc.invalidateQueries({ queryKey: ['creceipts'] });
    },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'sell failed'),
  });

  const openShift = useMutation({
    mutationFn: async () => (await api.post('/shifts/open', { cash_register_id: Number(regId), start_cash: 0 })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['shift'] }),
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'open failed'),
  });

  const closeShift = useMutation({
    mutationFn: async () => (await api.post(`/shifts/${shift.data.id}/close`, {})).data,
    onSuccess: (z) => {
      setMsg(`Смена закрыта. Z: выручка ${z.cash_sales + z.card_sales}, расхождение ${z.discrepancy}`);
      qc.invalidateQueries({ queryKey: ['shift'] });
    },
    onError: (e: any) => setMsg(e.response?.data?.error ?? 'close failed'),
  });

  return (
    <div className="p-6 max-w-5xl mx-auto grid grid-cols-2 gap-4">
      <div className="col-span-2 flex items-center gap-3">
        <h1 className="text-xl font-bold">Касса</h1>
        <select className="border rounded px-2 py-1 text-sm" value={regId} onChange={(e) => setRegId(e.target.value)}>
          {regs.data?.map((r: any) => (
            <option key={r.id} value={r.id}>{r.reg_number} ({r.model})</option>
          ))}
        </select>
        {shift.data ? (
          <span className="text-sm text-green-700">Смена №{shift.data.shift_number} открыта</span>
        ) : (
          <button className="px-3 py-1 bg-slate-900 text-white rounded text-sm" onClick={() => openShift.mutate()}>
            Открыть смену
          </button>
        )}
        {shift.data && (
          <button className="px-3 py-1 border rounded text-sm" onClick={() => closeShift.mutate()}>
            Закрыть смену (Z)
          </button>
        )}
        {ofdActive && (
          <span className="text-xs px-2 rounded bg-slate-100" title={ofdActive.name}>
            ОФД: {ofdActive.code === 'OFD_EMULATOR' ? 'эмулятор' : ofdActive.name}
          </span>
        )}
        <Link className="underline text-sm ml-auto" to="/me">Кабинет</Link>
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Сканирование / поиск</h2>
        <div className="flex gap-1">
          <input
            className="border rounded px-2 py-1 text-sm flex-1"
            placeholder="SKU / GTIN"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && addByCode()}
          />
          <button className="px-3 py-1 border rounded text-sm" onClick={addByCode}>+</button>
        </div>
        {cart.map((l) => (
          <div key={l.product_id} className="text-sm border-b py-1">
            <div className="flex justify-between">
              <span>{l.name} × {l.qty}</span>
              <span>{(l.price * l.qty).toFixed(2)}</span>
            </div>
            {l.is_marked && (
              <input
                className="border rounded px-2 py-1 text-xs w-full mt-1"
                placeholder={`Коды маркировки (${l.qty} шт через пробел)`}
                value={l.codes}
                onChange={(e) =>
                  setCart((c) => c.map((x) => (x.product_id === l.product_id ? { ...x, codes: e.target.value } : x)))
                }
              />
            )}
          </div>
        ))}
        <p className="font-bold mt-2">Итого: {total.toFixed(2)}</p>
        <div className="flex gap-2 mt-2 text-sm">
          <select className="border rounded px-2 py-1" value={payType} onChange={(e) => setPayType(e.target.value)}>
            <option value="CASH">Наличные</option>
            <option value="CARD">Карта</option>
            <option value="MIXED">Смешанная</option>
          </select>
          {payType !== 'CARD' && (
            <input className="border rounded px-2 py-1 w-28" placeholder="Получено" value={cash} onChange={(e) => setCash(e.target.value)} />
          )}
          <button
            className="px-4 py-1 bg-green-700 text-white rounded disabled:opacity-40"
            disabled={cart.length === 0 || !shift.data}
            onClick={() => sell.mutate()}
          >
            Пробить
          </button>
        </div>
        {msg && <p className="text-sm mt-2 text-slate-700">{msg}</p>}
      </div>

      <div className="border rounded p-4">
        <h2 className="font-bold mb-2 text-sm">Чеки смены (ОФД обновляется live)</h2>
        {receipts.data?.map((r: any) => (
          <div key={r.id} className="flex justify-between text-sm border-b py-1">
            <span>№{r.receipt_number} {r.receipt_type} — {r.total_amount}</span>
            <span className={r.ofd_status === 'COMPLETED' ? 'text-green-700' : r.ofd_status === 'FAILED' ? 'text-red-600' : 'text-amber-600'}>
              {r.ofd_status} {r.fiscal_sign?.slice(0, 8)}
            </span>
          </div>
        )) || <p className="text-sm text-slate-500">Нет открытой смены</p>}
      </div>
    </div>
  );
}
