import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ShoppingBag } from 'lucide-react';
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
