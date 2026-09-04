import { create } from 'zustand';

type UIState = {
  token: string | null;
  setToken: (t: string | null) => void;
};

export const useUI = create<UIState>((set) => ({
  token: localStorage.getItem('rms_token'),
  setToken: (token) => {
    if (token) localStorage.setItem('rms_token', token);
    else localStorage.removeItem('rms_token');
    set({ token });
  },
}));
