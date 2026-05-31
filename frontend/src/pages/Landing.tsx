import { motion } from 'framer-motion';
import { Link } from 'react-router-dom';
import {
  Activity,
  BarChart3,
  Bell,
  Boxes,
  CheckCircle2,
  Cloud,
  Database,
  Eye,
  Gauge,
  Github,
  RefreshCw,
  Rocket,
  ShoppingBag,
} from 'lucide-react';
import type { ReactNode } from 'react';

/* Scroll-reveal wrapper: fade + rise as the element enters the viewport
   (Railway-style render-in). */
function Reveal({ children, delay = 0 }: { children: ReactNode; delay?: number }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 24 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: '-80px' }}
      transition={{ duration: 0.7, ease: [0.215, 0.61, 0.355, 1], delay }}
    >
      {children}
    </motion.div>
  );
}

function Logo() {
  return (
    <div className="flex items-center gap-2">
      <div className="grid h-7 w-7 place-items-center rounded-md bg-gradient-to-br from-[#7E28BC] to-[#531AFF]">
        <ShoppingBag size={16} className="text-white" />
      </div>
      <span className="text-[17px] font-semibold tracking-tight">ShopHub</span>
    </div>
  );
}

function Nav() {
  const links = ['Product', 'Developers', 'Company', 'Pricing'];
  return (
    <header className="sticky top-0 z-50 border-b border-white/5 bg-bg/70 backdrop-blur">
      <nav className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
        <div className="flex items-center gap-10">
          <Logo />
          <ul className="hidden items-center gap-7 text-sm font-medium text-muted md:flex">
            {links.map((l) => (
              <li key={l} className="cursor-pointer transition-colors hover:text-fg">
                {l}
              </li>
            ))}
          </ul>
        </div>
        <div className="flex items-center gap-5 text-sm font-medium">
          <Link to="/login" className="text-fg/90 transition-colors hover:text-fg">
            Sign in
          </Link>
          <Link
            to="/login"
            className="rounded-lg border border-white/15 bg-white/5 px-4 py-2 transition-colors hover:bg-white/10"
          >
            Get started
          </Link>
        </div>
      </nav>
    </header>
  );
}

/* A ShopHub-flavoured take on Railway's architecture-canvas mockup. */
function DashboardMock() {
  const tabs = ['Architecture', 'Observability', 'Logs', 'Settings'];
  return (
    <div className="overflow-hidden rounded-xl border border-white/10 bg-[#0D0C14] shadow-[0px_100px_191px_rgba(62,45,45,0.24)]">
      {/* top bar */}
      <div className="flex items-center justify-between border-b border-white/[0.07] px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <div className="h-4 w-4 rounded bg-gradient-to-br from-[#7E28BC] to-[#531AFF]" />
          <span className="text-faint">/</span>
          <span>my-store</span>
          <span className="text-faint">/</span>
          <span className="text-muted">production</span>
        </div>
        <div className="hidden items-center gap-5 text-[13px] font-medium text-muted sm:flex">
          {tabs.map((t, i) => (
            <span key={t} className={i === 0 ? 'text-fg' : ''}>
              {t}
            </span>
          ))}
          <div className="h-6 w-6 rounded-full bg-gradient-to-br from-[#7E28BC] to-[#531AFF]" />
        </div>
      </div>
      {/* canvas */}
      <div className="dot-grid relative grid gap-4 p-6 sm:grid-cols-2">
        <ServiceCard icon={<ShoppingBag size={15} className="text-accent-bright" />} name="storefront" status="Deployed just now" />
        <ServiceCard icon={<Github size={15} />} name="backend" status="Just deployed via GitHub" />
        <ServiceCard icon={<Database size={15} className="text-[#4f8bd6]" />} name="postgres" status="Just deployed" sub="pg-data" />
        <ServiceCard icon={<Boxes size={15} className="text-[#d36cbf]" />} name="orders" status="Deployed just now" />
      </div>
    </div>
  );
}

function ServiceCard({
  icon,
  name,
  status,
  sub,
}: {
  icon: ReactNode;
  name: string;
  status: string;
  sub?: string;
}) {
  return (
    <div className="rounded-lg border border-line bg-card">
      <div className="flex items-center gap-2 px-4 pt-3 text-sm font-semibold">
        {icon}
        {name}
      </div>
      <div className="flex items-center gap-2 px-4 py-3 text-[13px] text-muted">
        <CheckCircle2 size={14} className="text-[#42946E]" />
        {status}
      </div>
      {sub && (
        <div className="flex items-center gap-2 border-t border-line px-4 py-2 text-[13px] text-faint">
          <Database size={13} /> {sub}
        </div>
      )}
    </div>
  );
}

/* Animated data-transfer panel: green particles continuously stream between two
   service nodes (Railway's "Instant networking" card). Loops forever. */
function DataFlow() {
  const dots = Array.from({ length: 40 }, (_, i) => ({
    left: 12 + ((i * 37) % 76),
    top: ((i * 53) % 90),
    delay: (i % 8) * 0.18,
    dur: 1.1 + (i % 6) * 0.18,
  }));
  return (
    <div className="rounded-xl border border-white/10 bg-card/60 p-6">
      <div className="rounded-lg border border-line bg-card p-4">
        <div className="flex items-center gap-2.5 font-semibold">
          <span className="relative flex h-3 w-3">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-teal-400 opacity-60" />
            <span className="relative inline-flex h-3 w-3 rounded-full bg-teal-400" />
          </span>
          shop-api
        </div>
        <div className="mt-1 pl-[22px] text-xs text-muted">api-prod.shophub.app</div>
        <div className="mt-3 flex items-center gap-2 text-[13px] text-muted">
          <CheckCircle2 size={14} className="text-[#42946E]" /> Online
        </div>
      </div>

      <div className="relative my-2 h-24">
        {dots.map((d, i) => (
          <motion.span
            key={i}
            className="absolute h-1.5 w-1.5 rounded-[2px] bg-[#3ECF8E]"
            style={{ left: `${d.left}%`, top: `${d.top}%` }}
            animate={{ opacity: [0.12, 0.9, 0.12], y: [4, -6, 4] }}
            transition={{ duration: d.dur, delay: d.delay, repeat: Infinity, ease: 'easeInOut' }}
          />
        ))}
        <div className="absolute left-1/2 top-1/2 flex -translate-x-1/2 -translate-y-1/2 items-center gap-2 whitespace-nowrap rounded-full border border-[#2c6b52] bg-[#11392b] px-3 py-1.5 font-mono text-[12px] text-[#6FD5B7]">
          <CheckCircle2 size={13} className="text-[#42946E]" />
          <span className="font-semibold text-fg">TCP:5432</span>
          <span className="text-faint">·</span> Private
          <span className="text-faint">·</span> Encrypted
          <span className="text-faint">·</span> &lt;1ms
        </div>
      </div>

      <div className="rounded-lg border border-line bg-card p-4">
        <div className="flex items-center gap-2.5 font-semibold">
          <Database size={15} className="text-[#4f8bd6]" /> postgres
        </div>
        <div className="mt-3 flex items-center gap-2 text-[13px] text-muted">
          <CheckCircle2 size={14} className="text-[#42946E]" /> 2 days ago via CLI
        </div>
      </div>
    </div>
  );
}

/* "Scale" panel: a service card over rows of load bars that shimmer
   continuously — the "handle more load / add replicas" feel. */
function ScalePanel() {
  return (
    <div className="rounded-xl border border-white/10 bg-card/60 p-6">
      <div className="rounded-lg border border-line bg-card p-5">
        <div className="flex items-center gap-2 font-semibold">
          <Rocket size={15} className="text-accent-bright" /> backend
        </div>
        <div className="mt-2 flex items-center gap-2 text-[13px] text-muted">
          <CheckCircle2 size={14} className="text-[#42946E]" /> Online · 3 replicas
        </div>
        <div className="mt-5 space-y-2.5">
          {[0, 1, 2].map((row) => (
            <div key={row} className="flex gap-1.5">
              {Array.from({ length: 14 }).map((_, i) => (
                <motion.span
                  key={i}
                  className="h-7 flex-1 rounded-sm bg-[#42946E]/40"
                  animate={{ opacity: [0.25, 0.85, 0.25] }}
                  transition={{
                    duration: 1.6,
                    delay: ((row * 14 + i) * 0.06) % 1.6,
                    repeat: Infinity,
                    ease: 'easeInOut',
                  }}
                />
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function Hero() {
  return (
    <section className="relative overflow-hidden px-6 pt-32 pb-20 text-center">
      {/* purple glow */}
      <div className="pointer-events-none absolute inset-x-0 top-0 h-[520px] bg-[radial-gradient(60%_60%_at_50%_0%,rgba(133,59,206,0.22),transparent_70%)]" />
      <div className="dot-grid pointer-events-none absolute inset-0 opacity-40" />
      <div className="relative mx-auto max-w-3xl">
        <motion.h1
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.7, ease: [0.215, 0.61, 0.355, 1] }}
          className="font-serif text-[42px] font-medium leading-[1.12] tracking-tightest text-fg sm:text-[56px]"
        >
          Ship your store peacefully
        </motion.h1>
        <motion.p
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.7, delay: 0.1, ease: [0.215, 0.61, 0.355, 1] }}
          className="mx-auto mt-6 max-w-[640px] text-[19px] leading-7 text-fg/70 sm:text-[20px]"
        >
          The all-in-one platform to launch, scale, and monitor your storefront —
          define a shop, we handle the rest.
        </motion.p>
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.7, delay: 0.2, ease: [0.215, 0.61, 0.355, 1] }}
          className="mt-8 flex flex-wrap items-center justify-center gap-3"
        >
          <Link
            to="/login"
            className="rounded-lg bg-accent px-6 py-3 text-[18px] font-medium text-white transition-[filter] hover:brightness-110"
          >
            Get started →
          </Link>
          <a
            href="#features"
            className="rounded-lg border border-white/25 bg-black/35 px-6 py-3 text-[18px] font-medium text-white transition-colors hover:bg-white/10"
          >
            Live demo
          </a>
        </motion.div>
      </div>
      <motion.div
        initial={{ opacity: 0, y: 40 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.9, delay: 0.35, ease: [0.215, 0.61, 0.355, 1] }}
        className="relative mx-auto mt-20 max-w-5xl"
      >
        <DashboardMock />
      </motion.div>
    </section>
  );
}

type Feat = {
  pill: string;
  title: ReactNode;
  body: string;
  points: { icon: ReactNode; title: string; body: string }[];
};

const FEATURES: Feat[] = [
  {
    pill: 'Launch and deploy',
    title: (
      <>
        Launch anything
        <br />
        without the ops
      </>
    ),
    body: 'Define a Shop and the operator provisions its database, deployment, ingress and monitoring. No YAML to learn.',
    points: [
      { icon: <Eye size={18} />, title: 'See your whole store', body: 'A visual canvas that makes your entire stack visible at a glance.' },
      { icon: <Cloud size={18} />, title: 'Correct config, every time', body: 'The operator reconciles desired state — no drift, no surprises.' },
      { icon: <Boxes size={18} />, title: 'Postgres or MongoDB', body: 'Pick a database per shop; we wire the connection automatically.' },
    ],
  },
  {
    pill: 'Observe',
    title: (
      <>
        Metrics, logs, and alerts
        <br />
        in one place
      </>
    ),
    body: 'Per-shop Grafana dashboards, Loki logs and Tempo traces — with Discord alerts the moment something degrades.',
    points: [
      { icon: <BarChart3 size={18} />, title: 'Metrics dashboard', body: 'Requests, latency and traffic per shop, out of the box.' },
      { icon: <Bell size={18} />, title: 'Alerts that reach you', body: 'Discord notifications the moment your conditions are met.' },
      { icon: <Activity size={18} />, title: 'Contextual logs', body: 'All logs in one place — spot issues without switching tools.' },
    ],
  },
  {
    pill: 'Scale and grow',
    title: (
      <>
        Grow big
        <br />
        without the growing pains
      </>
    ),
    body: 'Take a single instance to high availability. Scale replicas with one command, or let an HPA do it for you.',
    points: [
      { icon: <Gauge size={18} />, title: 'Scale on demand', body: 'kubectl scale shop, or autoscale on CPU — your choice.' },
      { icon: <Rocket size={18} />, title: 'High availability', body: 'Flip to high availability and run more replicas instantly.' },
    ],
  },
];

function FeatureSection({ feat, flip, panel }: { feat: Feat; flip: boolean; panel: ReactNode }) {
  const text = (
    <div>
      <span className="inline-block rounded-md bg-white/5 px-2.5 py-1 text-xs font-medium text-muted">
        {feat.pill}
      </span>
      <h2 className="mt-4 font-serif text-[34px] font-medium leading-[1.1] tracking-tight text-fg">
        {feat.title}
      </h2>
      <p className="mt-4 max-w-md text-[17px] leading-7 text-fg/65">{feat.body}</p>
      <a href="#" className="mt-4 inline-block text-[15px] font-medium text-fg">
        Learn more →
      </a>
      <div className="mt-8 space-y-5 border-t border-white/10 pt-6">
        {feat.points.map((p) => (
          <div key={p.title} className="flex gap-3">
            <span className="mt-0.5 text-accent-bright">{p.icon}</span>
            <div>
              <div className="text-[15px] font-semibold text-fg">{p.title}</div>
              <div className="text-[14px] leading-6 text-fg/55">{p.body}</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
  return (
    <Reveal>
      <div className="mx-auto grid max-w-6xl items-center gap-12 px-6 py-20 md:grid-cols-2">
        {flip ? (
          <>
            {panel}
            {text}
          </>
        ) : (
          <>
            {text}
            {panel}
          </>
        )}
      </div>
    </Reveal>
  );
}

const TESTIMONIALS = [
  {
    name: 'Kartik Aggarwal',
    role: 'Tech Lead at Bilt',
    quote:
      'On our last big sale we saw 1,500+ requests per second all fulfilled in under 50ms. Our team is really impressed with scale like that.',
  },
  {
    name: 'Daniel Lobaton',
    role: 'CTO at G2X',
    quote:
      'Stores that took a week to configure elsewhere take a day on ShopHub. If you understand basic Kubernetes, you are all set.',
  },
  {
    name: 'Daniel Moretti',
    role: 'Co-Founder & CTO at Mappa',
    quote:
      'ShopHub gives us instant observability into every storefront and makes spinning up new ones almost effortless.',
  },
];

function Testimonials() {
  return (
    <section className="bg-[#1a1320] px-6 py-24">
      <Reveal>
        <div className="mx-auto max-w-2xl text-center">
          <h2 className="font-serif text-[38px] font-medium text-fg">Trusted by the best in business</h2>
          <p className="mt-4 text-[17px] text-fg/55">
            ShopHub supports great teams wherever they are. Hear from some of the people building on it.
          </p>
        </div>
      </Reveal>
      <div className="mx-auto mt-14 grid max-w-6xl gap-6 md:grid-cols-3">
        {TESTIMONIALS.map((t, i) => (
          <Reveal key={t.name} delay={i * 0.08}>
            <div className="flex h-full flex-col justify-between rounded-xl border border-white/10 bg-white/[0.02] p-7">
              <p className="text-[15px] leading-7 text-fg/85">"{t.quote}"</p>
              <div className="mt-8 flex items-center gap-3">
                <div className="h-9 w-9 rounded-full bg-gradient-to-br from-[#7E28BC] to-[#531AFF]" />
                <div>
                  <div className="text-sm font-semibold">{t.name}</div>
                  <div className="text-xs text-muted">{t.role}</div>
                </div>
              </div>
            </div>
          </Reveal>
        ))}
      </div>
    </section>
  );
}

function CTA() {
  return (
    <section className="relative overflow-hidden px-6 py-32 text-center">
      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-[400px] bg-[radial-gradient(50%_80%_at_50%_100%,rgba(133,59,206,0.25),transparent_70%)]" />
      <Reveal>
        <div className="relative mx-auto flex max-w-md flex-col items-center gap-8">
          <div>
            <h2 className="font-serif text-[40px] font-semibold leading-[1.2] text-fg">
              Your storefront is
              <br />
              now boarding
            </h2>
            <p className="mt-4 text-[20px] text-fg/55">Launch your first shop today.</p>
          </div>
          <Link
            to="/login"
            className="btn-gradient inline-flex h-14 items-center rounded-[28px] px-8 text-[20px] font-medium"
          >
            Get started
          </Link>
        </div>
      </Reveal>
    </section>
  );
}

const FOOTER: Record<string, string[]> = {
  Product: ['Features', 'Pricing', 'Changelog', 'Templates'],
  Developers: ['Docs', 'API', 'Status', 'Blog'],
  Company: ['About', 'Careers', 'Contact', 'Legal'],
};

function Footer() {
  return (
    <footer className="border-t border-white/10 px-6 py-16">
      <div className="mx-auto grid max-w-6xl gap-10 md:grid-cols-[2fr_1fr_1fr_1fr]">
        <div>
          <Logo />
          <div className="mt-4 flex items-center gap-2 font-mono text-[13px] text-[#42946E]">
            <RefreshCw size={13} /> All systems operational
          </div>
        </div>
        {Object.entries(FOOTER).map(([group, items]) => (
          <div key={group}>
            <div className="text-sm font-semibold text-fg">{group}</div>
            <ul className="mt-4 space-y-3 text-sm text-muted">
              {items.map((it) => (
                <li key={it} className="cursor-pointer transition-colors hover:text-fg">
                  {it}
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <div className="mx-auto mt-12 max-w-6xl border-t border-white/10 pt-6 text-sm text-faint">
        © 2026 ShopHub. A DevOps faculty project.
      </div>
    </footer>
  );
}

export default function Landing() {
  return (
    <div className="min-h-screen bg-bg">
      <Nav />
      <Hero />
      <div id="features">
        {FEATURES.map((f, i) => (
          <FeatureSection
            key={f.pill}
            feat={f}
            flip={i % 2 === 1}
            panel={
              i === 1 ? (
                <DataFlow />
              ) : i === 2 ? (
                <ScalePanel />
              ) : (
                <div className="rounded-xl border border-white/10 bg-card/60 p-4">
                  <DashboardMock />
                </div>
              )
            }
          />
        ))}
      </div>
      <Testimonials />
      <CTA />
      <Footer />
    </div>
  );
}
