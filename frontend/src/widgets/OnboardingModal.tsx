import { useEffect, useState } from "react";
import { Check, ChevronLeft, ChevronRight, Music2, X } from "lucide-react";
import { getSetting, setSetting } from "../shared/api/client";

const ONBOARDING_KEY = "ui.onboardingCompleted";

const steps = [
  { icon: Music2, title: "Добро пожаловать в Cryon", text: "Ваше личное пространство для локальной музыки и любимых сервисов." },
  { icon: Music2, title: "Добавьте музыку", text: "Откройте Настройки и добавьте папку с треками. Cryon найдёт файлы, метаданные и обложки." },
  { icon: Music2, title: "Подключите сервисы", text: "В Настройках можно подключить Yandex Music, Spotify, SoundCloud и Last.fm для поиска и рекомендаций." },
  { icon: Music2, title: "Слушайте и собирайте", text: "Ищите музыку, добавляйте треки в избранное и плейлисты, а Cryon запомнит историю и предложит похожее." },
];

export function OnboardingModal() {
  const [open, setOpen] = useState(false);
  const [step, setStep] = useState(0);

  useEffect(() => {
    void getSetting(ONBOARDING_KEY).then((value) => {
      if (value !== "true") setOpen(true);
    });
    const show = () => { setStep(0); setOpen(true); };
    window.addEventListener("cryon:show-onboarding", show);
    return () => window.removeEventListener("cryon:show-onboarding", show);
  }, []);

  const close = () => {
    void setSetting(ONBOARDING_KEY, "true");
    setOpen(false);
  };

  if (!open) return null;
  const current = steps[step];
  const Icon = current.icon;
  const isLast = step === steps.length - 1;
  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm">
      <section role="dialog" aria-modal="true" aria-labelledby="onboarding-title" className="w-full max-w-lg rounded-3xl border border-white/15 bg-[#10121e] p-6 shadow-2xl sm:p-8">
        <div className="flex justify-between"><div className="text-sm text-slate-400">Знакомство · {step + 1} из {steps.length}</div><button aria-label="Закрыть обучение" onClick={close} className="text-slate-400 hover:text-white"><X size={20}/></button></div>
        <div className="my-8 flex h-16 w-16 items-center justify-center rounded-2xl bg-[color-mix(in_srgb,var(--app-accent)_25%,transparent)] text-[var(--app-accent)]"><Icon size={32}/></div>
        <h2 id="onboarding-title" className="text-2xl font-bold text-white">{current.title}</h2>
        <p className="mt-3 min-h-12 text-base leading-6 text-slate-300">{current.text}</p>
        <div className="mt-8 flex gap-2">{steps.map((_, index) => <span key={index} className={`h-1.5 flex-1 rounded-full ${index <= step ? "bg-[var(--app-accent)]" : "bg-white/10"}`}/>)}</div>
        <div className="mt-6 flex items-center justify-between"><button onClick={() => setStep((value) => Math.max(0, value - 1))} disabled={!step} className="flex items-center gap-1 text-sm text-slate-300 disabled:invisible"><ChevronLeft size={17}/>Назад</button><button onClick={() => isLast ? close() : setStep((value) => value + 1)} className="flex items-center gap-1 rounded-xl bg-[var(--app-accent)] px-4 py-2 text-sm font-semibold text-slate-950"><>{isLast ? <Check size={17}/> : null}{isLast ? "Начать" : "Далее"}{!isLast ? <ChevronRight size={17}/> : null}</></button></div>
      </section>
    </div>
  );
}