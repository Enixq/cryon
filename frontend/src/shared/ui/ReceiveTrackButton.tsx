import { Link2 } from "lucide-react";
import { useUiStore } from "../../store/uiStore";

export function ReceiveTrackButton() {
  const openShare = useUiStore((state) => state.openShare);
  return (
    <button
      type="button"
      onClick={() => openShare(null)}
      className="flex min-h-11 items-center justify-center gap-2 rounded-xl border border-white/10 bg-white/[.04] px-4 py-2 text-sm font-medium text-slate-200 transition-colors hover:bg-white/10 active:bg-white/15"
    >
      <Link2 size={18} /> Получить трек по ссылке
    </button>
  );
}
