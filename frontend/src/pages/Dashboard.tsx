import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Database, ExternalLink, KeyRound, LineChart, Pencil, Plus, ShoppingBag, Trash2, Wand2, X } from 'lucide-react';
import { api, token, type AdminCredentials, type EditShop, type GrafanaAccess, type NewShop, type Shop } from '../lib/api';

function Badge({ children, tone = 'default' }: { children: React.ReactNode; tone?: 'default' | 'accent' }) {
  const cls = tone === 'accent' ? 'bg-accent/20 text-accent-bright' : 'bg-white/5 text-muted';
  return <span className={`rounded-md px-2 py-0.5 text-xs font-medium ${cls}`}>{children}</span>;
}

function ShopCard({
  shop,
  onEdit,
  onDelete,
  onAdmin,
}: {
  shop: Shop;
  onEdit: (s: Shop) => void;
  onDelete: (n: string) => void;
  onAdmin: (s: Shop) => void;
}) {
  return (
    <div className="group rounded-xl border border-line bg-card p-5 transition-colors hover:border-white/20">
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2">
          <div className="grid h-8 w-8 place-items-center rounded-md bg-gradient-to-br from-[#7E28BC] to-[#531AFF]">
            <ShoppingBag size={15} className="text-white" />
          </div>
          <div>
            <div className="font-semibold">{shop.title}</div>
            <div className="text-xs text-muted">{shop.name}</div>
          </div>
        </div>
        <div className="flex items-center gap-3">
          {shop.url && (
            <a
              href={shop.url}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-xs font-medium text-accent-bright hover:underline"
              title="Open storefront"
            >
              Open <ExternalLink size={13} />
            </a>
          )}
          <button
            onClick={() => onAdmin(shop)}
            className="text-faint opacity-0 transition-opacity hover:text-accent-bright group-hover:opacity-100"
            title="Shop admin credentials"
          >
            <KeyRound size={15} />
          </button>
          <button
            onClick={() => onEdit(shop)}
            className="text-faint opacity-0 transition-opacity hover:text-accent-bright group-hover:opacity-100"
            title="Edit shop"
          >
            <Pencil size={15} />
          </button>
          <button
            onClick={() => onDelete(shop.name)}
            className="text-faint opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
            title="Delete shop"
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Badge tone={shop.availability === 'high' ? 'accent' : 'default'}>
          {shop.availability === 'high' ? 'High availability' : 'Standard'}
        </Badge>
        <Badge>
          <span className="inline-flex items-center gap-1">
            <Database size={11} /> {shop.database}
          </span>
        </Badge>
      </div>
    </div>
  );
}

const EMPTY: NewShop = {
  name: '',
  title: '',
  availability: 'standard',
  database: 'postgres',
  walletAddress: '',
};

function CreateModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [form, setForm] = useState<NewShop>(EMPTY);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [generating, setGenerating] = useState(false);

  // Provision a Wallet CR via the operator and fill in the resulting address.
  async function generateWallet() {
    setGenerating(true);
    setError(null);
    try {
      const w = await api.createWallet();
      if (w.address) {
        setForm((f) => ({ ...f, walletAddress: w.address! }));
      } else {
        setError(w.error ?? 'Wallet is being provisioned — try again shortly.');
      }
    } catch (err) {
      setError((err as Error).message.replace(/^\d+ [^:]+:\s*/, ''));
    } finally {
      setGenerating(false);
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.createShop(form);
      onCreated();
      onClose();
    } catch (err) {
      setError((err as Error).message.replace(/^\d+ [^:]+:\s*/, ''));
    } finally {
      setBusy(false);
    }
  }

  const field = 'w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm outline-none focus:border-accent-bright';

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/60 p-6 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-2xl border border-white/10 bg-card p-6">
        <div className="flex items-center justify-between">
          <h2 className="font-serif text-xl font-medium">New shop</h2>
          <button onClick={onClose} className="text-faint hover:text-fg">
            <X size={18} />
          </button>
        </div>
        <form onSubmit={submit} className="mt-5 space-y-4">
          <div>
            <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Name (DNS-safe)</label>
            <input
              required
              pattern="[a-z0-9-]+"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="my-store"
              className={field}
            />
          </div>
          <div>
            <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Title</label>
            <input
              required
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              placeholder="My Store"
              className={field}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Availability</label>
              <select
                value={form.availability}
                onChange={(e) => setForm({ ...form, availability: e.target.value as NewShop['availability'] })}
                className={field}
              >
                <option value="standard">Standard</option>
                <option value="high">High</option>
              </select>
            </div>
            <div>
              <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Database</label>
              <select
                value={form.database}
                onChange={(e) => setForm({ ...form, database: e.target.value as NewShop['database'] })}
                className={field}
              >
                <option value="postgres">Postgres</option>
                <option value="mongodb">MongoDB</option>
              </select>
            </div>
          </div>
          <div>
            <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Wallet address</label>
            <div className="flex gap-2">
              <input
                required
                value={form.walletAddress}
                onChange={(e) => setForm({ ...form, walletAddress: e.target.value })}
                placeholder="0x… (where buyers send payment)"
                className={field}
              />
              <button
                type="button"
                onClick={generateWallet}
                disabled={generating}
                title="Generate a wallet for me"
                className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-line px-3 text-[13px] font-medium text-muted transition-colors hover:text-fg disabled:opacity-60"
              >
                <Wand2 size={14} /> {generating ? '…' : 'Generate'}
              </button>
            </div>
          </div>
          <label className="flex items-center gap-2.5 text-sm text-muted">
            <input
              type="checkbox"
              checked={form.discordChannel ?? false}
              onChange={(e) => setForm({ ...form, discordChannel: e.target.checked })}
              className="h-4 w-4 rounded border-line bg-surface accent-[#531AFF]"
            />
            Create a Discord notification channel for this shop's alerts
          </label>
          {error && <p className="text-sm text-red-400">{error}</p>}
          <button
            type="submit"
            disabled={busy}
            className="btn-gradient h-11 w-full rounded-lg text-[15px] font-medium disabled:opacity-60"
          >
            {busy ? 'Creating…' : 'Create shop'}
          </button>
        </form>
      </div>
    </div>
  );
}

function EditModal({ shop, onClose, onSaved }: { shop: Shop; onClose: () => void; onSaved: () => void }) {
  const [form, setForm] = useState({
    title: shop.title,
    availability: shop.availability,
    walletAddress: shop.walletAddress ?? '',
  });
  const [enableDiscord, setEnableDiscord] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const patch: EditShop = { ...form };
      if (enableDiscord) patch.discordChannel = true;
      await api.updateShop(shop.name, patch);
      onSaved();
      onClose();
    } catch (err) {
      setError((err as Error).message.replace(/^\d+ [^:]+:\s*/, ''));
    } finally {
      setBusy(false);
    }
  }

  const field = 'w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm outline-none focus:border-accent-bright';

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/60 p-6 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-2xl border border-white/10 bg-card p-6">
        <div className="flex items-center justify-between">
          <h2 className="font-serif text-xl font-medium">Edit {shop.name}</h2>
          <button onClick={onClose} className="text-faint hover:text-fg">
            <X size={18} />
          </button>
        </div>
        <form onSubmit={submit} className="mt-5 space-y-4">
          <div>
            <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Title</label>
            <input
              required
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              className={field}
            />
          </div>
          <div>
            <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Availability</label>
            <select
              value={form.availability}
              onChange={(e) => setForm({ ...form, availability: e.target.value as Shop['availability'] })}
              className={field}
            >
              <option value="standard">Standard</option>
              <option value="high">High</option>
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-[13px] font-medium text-fg/80">Wallet address</label>
            <input
              required
              value={form.walletAddress}
              onChange={(e) => setForm({ ...form, walletAddress: e.target.value })}
              placeholder="0x… (where buyers send payment)"
              className={field}
            />
          </div>
          {shop.discordChannel ? (
            <p className="text-xs text-faint">Discord notification channel is enabled for this shop.</p>
          ) : (
            <label className="flex items-center gap-2.5 text-sm text-muted">
              <input
                type="checkbox"
                checked={enableDiscord}
                onChange={(e) => setEnableDiscord(e.target.checked)}
                className="h-4 w-4 rounded border-line bg-surface accent-[#531AFF]"
              />
              Enable a Discord notification channel for this shop's alerts
            </label>
          )}
          <p className="text-xs text-faint">Database ({shop.database}) is fixed at creation and can't be changed.</p>
          {error && <p className="text-sm text-red-400">{error}</p>}
          <button
            type="submit"
            disabled={busy}
            className="btn-gradient h-11 w-full rounded-lg text-[15px] font-medium disabled:opacity-60"
          >
            {busy ? 'Saving…' : 'Save changes'}
          </button>
        </form>
      </div>
    </div>
  );
}

// AdminCredsModal shows the operator-generated password for the shop's own
// admin dashboard (storefront /admin), plus a direct link to the login page.
function AdminCredsModal({ shop, onClose }: { shop: Shop; onClose: () => void }) {
  const [creds, setCreds] = useState<AdminCredentials | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .adminCredentials(shop.name)
      .then(setCreds)
      .catch((e) => setError((e as Error).message.replace(/^\d+ [^:]+:\s*/, '')));
  }, [shop.name]);

  const row = 'flex items-center justify-between gap-3 rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm';

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/60 p-6 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-2xl border border-white/10 bg-card p-6">
        <div className="flex items-center justify-between">
          <h2 className="font-serif text-xl font-medium">Shop admin — {shop.name}</h2>
          <button onClick={onClose} className="text-faint hover:text-fg">
            <X size={18} />
          </button>
        </div>
        <p className="mt-2 text-sm text-muted">
          Use this password to sign into your storefront's admin dashboard and manage items and orders.
        </p>
        {error && <p className="mt-4 text-sm text-red-400">{error}</p>}
        {creds && (
          <div className="mt-5 space-y-2.5">
            <div className={row}>
              <span className="text-muted">Password</span>
              <code className="select-all text-fg">{creds.password}</code>
            </div>
            {creds.loginUrl && (
              <a
                href={creds.loginUrl}
                target="_blank"
                rel="noreferrer"
                className="btn-gradient mt-2 inline-flex h-11 w-full items-center justify-center gap-2 rounded-lg text-[15px] font-medium"
              >
                Open admin login <ExternalLink size={15} />
              </a>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// MetricsModal shows the tenant's own Grafana access: a link plus the scoped
// Viewer login that only sees this tenant's dashboards (spec 4.1 optional).
function MetricsModal({ onClose }: { onClose: () => void }) {
  const [info, setInfo] = useState<GrafanaAccess | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .grafana()
      .then(setInfo)
      .catch((e) => setError((e as Error).message.replace(/^\d+ [^:]+:\s*/, '')));
  }, []);

  const row = 'flex items-center justify-between gap-3 rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm';

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/60 p-6 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-2xl border border-white/10 bg-card p-6">
        <div className="flex items-center justify-between">
          <h2 className="font-serif text-xl font-medium">Your metrics</h2>
          <button onClick={onClose} className="text-faint hover:text-fg">
            <X size={18} />
          </button>
        </div>
        <p className="mt-2 text-sm text-muted">
          A private Grafana organization that shows only your shops' dashboards.
        </p>
        {error && <p className="mt-4 text-sm text-red-400">{error}</p>}
        {info && (
          <div className="mt-5 space-y-2.5">
            <div className={row}>
              <span className="text-muted">Username</span>
              <code className="text-fg">{info.login}</code>
            </div>
            <div className={row}>
              <span className="text-muted">Password</span>
              <code className="text-fg">{info.password}</code>
            </div>
            <div className={row}>
              <span className="text-muted">Organization</span>
              <code className="text-fg">{info.org}</code>
            </div>
            <a
              href={info.url}
              target="_blank"
              rel="noreferrer"
              className="btn-gradient mt-2 inline-flex h-11 w-full items-center justify-center gap-2 rounded-lg text-[15px] font-medium"
            >
              Open Grafana <ExternalLink size={15} />
            </a>
          </div>
        )}
      </div>
    </div>
  );
}

export default function Dashboard() {
  const [shops, setShops] = useState<Shop[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Shop | null>(null);
  const [adminFor, setAdminFor] = useState<Shop | null>(null);
  const [showMetrics, setShowMetrics] = useState(false);
  const navigate = useNavigate();

  function load() {
    api
      .listShops()
      .then(setShops)
      .catch((e) => setError((e as Error).message));
  }
  useEffect(load, []);

  function logout() {
    token.clear();
    navigate('/login');
  }

  async function remove(name: string) {
    await api.deleteShop(name);
    load();
  }

  return (
    <div className="min-h-screen bg-bg">
      <header className="sticky top-0 z-40 border-b border-white/5 bg-bg/70 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-2 text-sm font-medium">
            <div className="grid h-7 w-7 place-items-center rounded-md bg-gradient-to-br from-[#7E28BC] to-[#531AFF]">
              <ShoppingBag size={15} className="text-white" />
            </div>
            <span className="font-semibold">ShopHub</span>
            <span className="text-faint">/</span>
            <span className="text-muted">production</span>
          </div>
          <div className="flex items-center gap-5">
            <button
              onClick={() => setShowMetrics(true)}
              className="inline-flex items-center gap-1.5 text-sm font-medium text-muted transition-colors hover:text-fg"
            >
              <LineChart size={15} /> Metrics
            </button>
            <button onClick={logout} className="text-sm font-medium text-muted transition-colors hover:text-fg">
              Sign out
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-10">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="font-serif text-[28px] font-medium">Your shops</h1>
            <p className="mt-1 text-sm text-muted">Each shop is a live storefront with its own database and monitoring.</p>
          </div>
          <button
            onClick={() => setCreating(true)}
            className="inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-sm font-medium text-white transition-[filter] hover:brightness-110"
          >
            <Plus size={16} /> New shop
          </button>
        </div>

        {error && <p className="mt-6 text-sm text-red-400">{error}</p>}

        {shops.length === 0 && !error ? (
          <div className="mt-16 rounded-xl border border-dashed border-line py-20 text-center">
            <div className="mx-auto grid h-12 w-12 place-items-center rounded-xl bg-white/5">
              <ShoppingBag className="text-muted" />
            </div>
            <p className="mt-4 text-fg">No shops yet</p>
            <p className="mt-1 text-sm text-muted">Create your first storefront to get started.</p>
            <button
              onClick={() => setCreating(true)}
              className="mt-5 inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-sm font-medium text-white hover:brightness-110"
            >
              <Plus size={16} /> New shop
            </button>
          </div>
        ) : (
          <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {shops.map((s) => (
              <ShopCard key={s.name} shop={s} onEdit={setEditing} onDelete={remove} onAdmin={setAdminFor} />
            ))}
          </div>
        )}
      </main>

      {creating && <CreateModal onClose={() => setCreating(false)} onCreated={load} />}
      {editing && <EditModal shop={editing} onClose={() => setEditing(null)} onSaved={load} />}
      {adminFor && <AdminCredsModal shop={adminFor} onClose={() => setAdminFor(null)} />}
      {showMetrics && <MetricsModal onClose={() => setShowMetrics(false)} />}
    </div>
  );
}
