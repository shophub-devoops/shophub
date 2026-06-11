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
  // Opt-in: provision a Discord notification channel for this shop's alerts.
  discordChannel?: boolean;
};

// AdminCredentials is the operator-generated login for the shop's own admin
// dashboard (storefront /admin).
export type AdminCredentials = {
  password: string;
  loginUrl: string;
};

// EditShop is the mutable subset of a Shop. Database is fixed at creation
// (changing it would destroy data) so it isn't editable.
export type EditShop = {
  title?: string;
  availability?: 'standard' | 'high';
  walletAddress?: string;
};

export type GrafanaAccess = {
  url: string;
  login: string;
  password: string;
  org: string;
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
  // Web3 wallet sign-in (spec 1.1): fetch a nonce to sign, then submit the
  // signature for the backend to verify and issue a JWT.
  walletNonce: (address: string) =>
    request<{ nonce: string; message: string; token: string }>('/api/auth/nonce', {
      method: 'POST',
      body: JSON.stringify({ address }),
    }),
  walletLogin: (address: string, signature: string, nonceToken: string) =>
    request<AuthResponse>('/api/auth/wallet', {
      method: 'POST',
      body: JSON.stringify({ address, signature, token: nonceToken }),
    }),
  listShops: () => request<Shop[]>('/api/shops'),
  createShop: (shop: NewShop) =>
    request<Shop>('/api/shops', { method: 'POST', body: JSON.stringify(shop) }),
  updateShop: (name: string, patch: EditShop) =>
    request<Shop>(`/api/shops/${name}`, { method: 'PUT', body: JSON.stringify(patch) }),
  deleteShop: (name: string) =>
    request<void>(`/api/shops/${name}`, { method: 'DELETE' }),
  // Operator-generated admin login for the shop's own dashboard.
  adminCredentials: (name: string) =>
    request<AdminCredentials>(`/api/shops/${name}/admin-credentials`),
  // Generate a wallet via the operator's Wallet CRD; returns the address.
  createWallet: () =>
    request<{ name: string; address?: string; error?: string }>('/api/wallets', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  // Per-tenant Grafana access: link + scoped login showing only this tenant's
  // dashboards (spec 4.1 optional).
  grafana: () => request<GrafanaAccess>('/api/grafana'),
};
