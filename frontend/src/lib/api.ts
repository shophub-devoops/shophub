// Typed client for the ShopHub backend. JWT (from register/login) is kept in
// localStorage and sent as a Bearer token on shop requests.

const TOKEN_KEY = 'shophub_token';

export const token = {
  get: () => localStorage.getItem(TOKEN_KEY),
  set: (t: string) => localStorage.setItem(TOKEN_KEY, t),
  clear: () => localStorage.removeItem(TOKEN_KEY),
};

export type AuthResponse = { token: string; namespace: string };

export type Shop = {
  name: string;
  namespace: string;
  title: string;
  availability: 'standard' | 'high';
  database: 'postgres' | 'mongodb';
  walletAddress?: string;
  url?: string;
};

export type NewShop = {
  name: string;
  title: string;
  availability: 'standard' | 'high';
  database: 'postgres' | 'mongodb';
  walletAddress: string;
};

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init.headers as Record<string, string>),
  };
  const t = token.get();
  if (t) headers.Authorization = `Bearer ${t}`;

  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${body}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  register: (email: string, password: string) =>
    request<AuthResponse>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),
  login: (email: string, password: string) =>
    request<AuthResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),
  listShops: () => request<Shop[]>('/api/shops'),
  createShop: (shop: NewShop) =>
    request<Shop>('/api/shops', { method: 'POST', body: JSON.stringify(shop) }),
  deleteShop: (name: string) =>
    request<void>(`/api/shops/${name}`, { method: 'DELETE' }),
};
