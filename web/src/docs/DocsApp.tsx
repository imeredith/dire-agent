import { ArrowLeft, ArrowRight, CheckCircle2, FlaskConical, Menu, TerminalSquare, X } from "lucide-react";
import { useState } from "react";
import { currentFeature, featureDocs, featurePath } from "./features";
import type { FeatureDoc } from "./types";

const groups: FeatureDoc["group"][] = ["Core", "Agent controls", "Capabilities", "Operations"];

export function DocsApp() {
  const feature = currentFeature(window.location.pathname);
  const [navOpen, setNavOpen] = useState(false);
  return (
    <div className="min-h-dvh bg-[#080a0e] text-slate-100 selection:bg-[#ff7657]/30">
      {navOpen && <button className="fixed inset-0 z-40 bg-black/65 lg:hidden" aria-label="Close docs navigation" onClick={() => setNavOpen(false)} />}
      <aside className={`fixed inset-y-0 left-0 z-50 flex w-[286px] flex-col border-r border-white/10 bg-[#0c0f14] transition-transform lg:translate-x-0 ${navOpen ? "translate-x-0" : "-translate-x-full"}`}>
        <div className="flex h-16 items-center gap-3 border-b border-white/10 px-5">
          <span className="grid size-9 place-items-center rounded-xl border border-[#ff7657]/25 bg-[#ff7657]/10 text-[#ff7657]"><TerminalSquare size={18} /></span>
          <span className="grid leading-tight"><strong className="font-mono text-sm">Dire Agent docs</strong><small className="text-[10px] text-slate-500">Web UI verification guide</small></span>
          <button className="ml-auto grid size-8 place-items-center rounded-lg border border-white/10 text-slate-400 lg:hidden" onClick={() => setNavOpen(false)} aria-label="Close docs navigation"><X size={16} /></button>
        </div>
        <nav className="min-h-0 flex-1 overflow-y-auto px-3 py-5" aria-label="Feature documentation">
          <a href="/docs" className={`mb-5 flex rounded-lg px-3 py-2 text-xs font-semibold transition hover:bg-white/5 hover:text-white ${feature ? "text-slate-400" : "bg-white/[0.07] text-white"}`}>Feature index</a>
          {groups.map((group) => (
            <section className="mb-6" key={group}>
              <h2 className="mb-2 px-3 font-mono text-[9px] font-bold uppercase tracking-[0.16em] text-slate-600">{group}</h2>
              <div className="grid gap-0.5">
                {featureDocs.filter((item) => item.group === group).map((item) => (
                  <a key={item.slug} href={featurePath(item)} className={`rounded-lg px-3 py-2 text-[11px] transition hover:bg-white/5 hover:text-white ${feature?.slug === item.slug ? "bg-[#ff7657]/10 text-[#ff9a83] shadow-[inset_2px_0_#ff7657]" : "text-slate-400"}`}>{item.title}</a>
                ))}
              </div>
            </section>
          ))}
        </nav>
        <div className="border-t border-white/10 p-4"><a href="/" className="flex items-center gap-2 rounded-lg px-3 py-2 text-[11px] text-slate-400 transition hover:bg-white/5 hover:text-white"><ArrowLeft size={14} /> Back to agent UI</a></div>
      </aside>

      <div className="lg:pl-[286px]">
        <header className="sticky top-0 z-30 flex h-16 items-center border-b border-white/10 bg-[#080a0e]/85 px-4 backdrop-blur-xl sm:px-7 lg:px-10">
          <button className="mr-3 grid size-9 place-items-center rounded-lg border border-white/10 text-slate-400 lg:hidden" onClick={() => setNavOpen(true)} aria-label="Open docs navigation"><Menu size={18} /></button>
          <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-slate-500">Documentation / {feature?.group ?? "Features"}</span>
          <a href="/" className="ml-auto rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2 text-[10px] font-semibold text-slate-300 transition hover:border-white/20 hover:bg-white/[0.06]">Open app</a>
        </header>
        {feature ? <FeaturePage feature={feature} /> : <FeatureIndex />}
      </div>
    </div>
  );
}

function FeatureIndex() {
  return (
    <main className="mx-auto w-full max-w-6xl px-5 py-14 sm:px-8 lg:px-12 lg:py-20">
      <span className="font-mono text-[10px] font-bold uppercase tracking-[0.18em] text-[#ff7657]">Executable documentation</span>
      <h1 className="mt-5 max-w-4xl text-4xl font-semibold tracking-[-0.05em] text-white sm:text-5xl lg:text-6xl">Every feature, with a browser test you can repeat.</h1>
      <p className="mt-6 max-w-2xl text-sm leading-7 text-slate-400">These pages describe the behavior and the exact Web UI steps used to verify it against the local daemon. Start with connection, then work through the capability-specific fixtures.</p>
      <div className="mt-12 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {featureDocs.map((feature, index) => (
          <a href={featurePath(feature)} key={feature.slug} className="group flex min-h-48 flex-col rounded-2xl border border-white/10 bg-white/[0.025] p-5 transition hover:-translate-y-0.5 hover:border-[#ff7657]/30 hover:bg-white/[0.045]">
            <span className="font-mono text-[9px] uppercase tracking-[0.15em] text-slate-600">{String(index + 1).padStart(2, "0")} · {feature.group}</span>
            <h2 className="mt-5 text-lg font-semibold tracking-tight text-slate-100">{feature.title}</h2>
            <p className="mt-3 text-xs leading-6 text-slate-400">{feature.summary}</p>
            <span className="mt-auto flex items-center gap-2 pt-5 text-[10px] font-bold text-[#ff8e74]">Test this feature <ArrowRight className="transition-transform group-hover:translate-x-1" size={14} /></span>
          </a>
        ))}
      </div>
    </main>
  );
}

function FeaturePage({ feature }: { feature: FeatureDoc }) {
  return (
    <main className="mx-auto w-full max-w-4xl px-5 py-12 sm:px-8 lg:px-12 lg:py-16">
      <a href="/docs" className="inline-flex items-center gap-2 text-[10px] font-semibold text-slate-500 transition hover:text-slate-200"><ArrowLeft size={13} /> All features</a>
      <div className="mt-8 flex items-start gap-4">
        <span className="grid size-11 shrink-0 place-items-center rounded-2xl border border-[#ff7657]/25 bg-[#ff7657]/10 text-[#ff7657]"><FlaskConical size={19} /></span>
        <div><span className="font-mono text-[9px] font-bold uppercase tracking-[0.17em] text-[#ff7657]">{feature.group}</span><h1 className="mt-2 text-3xl font-semibold tracking-[-0.045em] text-white sm:text-5xl">{feature.title}</h1></div>
      </div>
      <p className="mt-7 max-w-3xl text-sm leading-7 text-slate-400">{feature.summary}</p>

      <section className="mt-12 rounded-2xl border border-white/10 bg-white/[0.025] p-5 sm:p-6">
        <h2 className="text-sm font-semibold text-white">Before you test</h2>
        <ul className="mt-4 grid gap-3">
          {feature.prerequisites.map((item) => <li key={item} className="flex gap-3 text-xs leading-6 text-slate-400"><CheckCircle2 className="mt-1 shrink-0 text-emerald-400" size={14} /><span><InlineCode text={item} /></span></li>)}
        </ul>
      </section>

      <section className="mt-12">
        <div className="flex items-end justify-between gap-5"><div><span className="font-mono text-[9px] font-bold uppercase tracking-[0.17em] text-slate-600">Web UI procedure</span><h2 className="mt-2 text-2xl font-semibold tracking-tight text-white">Test steps</h2></div><span className="rounded-full border border-white/10 px-3 py-1.5 font-mono text-[9px] text-slate-500">{feature.steps.length} checks</span></div>
        <ol className="mt-6 grid gap-4">
          {feature.steps.map((step, index) => (
            <li key={step.action} data-testid="feature-test-step" className="grid gap-4 rounded-2xl border border-white/10 bg-[#0d1015] p-5 sm:grid-cols-[36px_1fr]">
              <span className="grid size-9 place-items-center rounded-xl bg-[#ff7657]/10 font-mono text-[10px] font-bold text-[#ff8e74]">{index + 1}</span>
              <div><h3 className="text-xs font-semibold leading-6 text-slate-100"><InlineCode text={step.action} /></h3><div className="mt-3 border-l border-emerald-400/30 pl-4"><span className="font-mono text-[8px] font-bold uppercase tracking-[0.14em] text-emerald-400">Expected</span><p className="mt-1 text-[11px] leading-6 text-slate-400"><InlineCode text={step.expected} /></p></div></div>
            </li>
          ))}
        </ol>
      </section>

      {feature.notes?.length ? <section className="mt-10 rounded-2xl border border-amber-300/15 bg-amber-300/[0.04] p-5"><h2 className="font-mono text-[9px] font-bold uppercase tracking-[0.16em] text-amber-300">Notes</h2><ul className="mt-3 grid gap-2 text-[11px] leading-6 text-slate-400">{feature.notes.map((note) => <li key={note}>• {note}</li>)}</ul></section> : null}
    </main>
  );
}

function InlineCode({ text }: { text: string }) {
  const parts = text.split(/(`[^`]+`)/g);
  return <>{parts.map((part, index) => part.startsWith("`") && part.endsWith("`") ? <code key={index} className="rounded border border-white/10 bg-black/30 px-1.5 py-0.5 font-mono text-[0.9em] text-[#f3b7a9]">{part.slice(1, -1)}</code> : part)}</>;
}
