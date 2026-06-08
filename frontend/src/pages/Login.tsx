import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ShoppingBag, Wallet } from 'lucide-react';
import { api, token } from '../lib/api';

export default function Login() {
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = mode === 'login' ? await api.login(email, password) : await api.register(email, password);
      token.set(res.token);
      navigate('/dashboard');
    } catch (err) {
      setError((err as Error).message.replace(/^\d+ [^:]+:\s*/, '') || 'Something went wrong');
    } finally {
      setBusy(false);
    }
  }

  // Web3 sign-in (spec 1.1): connect MetaMask, sign a server nonce, log in.
  async function walletSignIn() {
    setBusy(true);
    setError(null);
    try {
      const eth = (window as { ethereum?: { request: (a: { method: string; params?: unknown[] }) => Promise<never> } }).ethereum;
      if (!eth) throw new Error('MetaMask not found — install the browser extension.');
      const accounts = (await eth.request({ method: 'eth_requestAccounts' })) as unknown as string[];
      const address = accounts[0];
      const { message } = await api.walletNonce(address);
      const signature = (await eth.request({ method: 'personal_sign', params: [message, address] })) as unknown as string;
      const res = await api.walletLogin(address, signature);
      token.set(res.token);
      navigate('/dashboard');
    } catch (err) {
      setError((err as Error).message.replace(/^\d+ [^:]+:\s*/, '') || 'Wallet sign-in failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="relative grid min-h-screen place-items-center overflow-hidden bg-bg px-6">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-[420px] bg-[radial-gradient(50%_60%_at_50%_0%,rgba(133,59,206,0.2),transparent_70%)]" />
      <div className="dot-grid pointer-events-none absolute inset-0 opacity-30" />

      <div className="relative w-full max-w-[400px]">
        <Link to="/" className="mb-8 flex items-center justify-center gap-2">
          <div className="grid h-8 w-8 place-items-center rounded-md bg-gradient-to-br from-[#7E28BC] to-[#531AFF]">
            <ShoppingBag size={17} className="text-white" />
          </div>
          <span className="text-lg font-semibold tracking-tight">ShopHub</span>
        </Link>

        <div className="rounded-2xl border border-white/10 bg-card/80 p-8 backdrop-blur">
          <h1 className="text-center font-serif text-[26px] font-medium">
            {mode === 'login' ? 'Welcome back' : 'Create your account'}
          </h1>
          <p className="mt-2 text-center text-sm text-muted">
            {mode === 'login' ? 'Sign in to manage your shops' : 'Start shipping stores in minutes'}
          </p>

          <form onSubmit={submit} className="mt-7 space-y-4">
            <div>
              <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Email</label>
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                className="w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm outline-none transition-colors placeholder:text-faint focus:border-accent-bright"
              />
            </div>
            <div>
              <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Password</label>
              <input
                type="password"
                required
                minLength={8}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                className="w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm outline-none transition-colors placeholder:text-faint focus:border-accent-bright"
              />
            </div>

            {error && <p className="text-sm text-red-400">{error}</p>}

            <button
              type="submit"
              disabled={busy}
              className="btn-gradient h-11 w-full rounded-lg text-[15px] font-medium disabled:opacity-60"
            >
              {busy ? 'Please wait…' : mode === 'login' ? 'Sign in' : 'Create account'}
            </button>
          </form>

          <div className="my-5 flex items-center gap-3 text-xs text-faint">
            <div className="h-px flex-1 bg-line" />
            or
            <div className="h-px flex-1 bg-line" />
          </div>

          <button
            onClick={walletSignIn}
            disabled={busy}
            className="flex h-11 w-full items-center justify-center gap-2 rounded-lg border border-line bg-surface text-[15px] font-medium text-fg transition-colors hover:border-accent-bright disabled:opacity-60"
          >
            <Wallet size={16} /> Continue with wallet
          </button>

          <p className="mt-6 text-center text-sm text-muted">
            {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
            <button
              onClick={() => {
                setMode(mode === 'login' ? 'register' : 'login');
                setError(null);
              }}
              className="font-medium text-accent-bright hover:underline"
            >
              {mode === 'login' ? 'Sign up' : 'Sign in'}
            </button>
          </p>
        </div>
      </div>
    </div>
  );
}
