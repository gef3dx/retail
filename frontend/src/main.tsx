import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { Me } from './pages/Me';
import { Users } from './pages/Users';
import { Orgs } from './pages/Orgs';
import { Products } from './pages/Products';
import { Dicts } from './pages/Dicts';
import { Cashier } from './pages/Cashier';
import { useAuth } from './store';
import './index.css';

const qc = new QueryClient();

function Guard({ children }: { children: JSX.Element }) {
  const access = useAuth((s) => s.access);
  if (!access) return <Navigate to="/login" replace />;
  return children;
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route path="/me" element={<Guard><Me /></Guard>} />
          <Route path="/users" element={<Guard><Users /></Guard>} />
          <Route path="/orgs" element={<Guard><Orgs /></Guard>} />
          <Route path="/products" element={<Guard><Products /></Guard>} />
          <Route path="/dicts" element={<Guard><Dicts /></Guard>} />
          <Route path="/cashier" element={<Guard><Cashier /></Guard>} />
          <Route path="*" element={<Navigate to="/me" replace />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
