import { create } from 'zustand';

type AuthState = {
  access: string | null;
  refresh: string | null;
  setTokens: (a: string | null, r: string | null) => void;
  logout: () => void;
};

function load(k: string): string | null {
  return localStorage.getItem(k);
}

export const useAuth = create<AuthState>((set) => ({
  access: load('rms_access'),
  refresh: load('rms_refresh'),
  setTokens: (access, refresh) => {
    if (access) localStorage.setItem('rms_access', access);
    else localStorage.removeItem('rms_access');
    if (refresh) localStorage.setItem('rms_refresh', refresh);
    else localStorage.removeItem('rms_refresh');
    set({ access, refresh });
  },
  logout: () => {
    localStorage.removeItem('rms_access');
    localStorage.removeItem('rms_refresh');
    localStorage.removeItem('rms_token');
    set({ access: null, refresh: null });
  },
}));
