import { Heart } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  listSavedCollections,
  removeSavedCollection,
  saveCollection,
  type SavedCollection,
} from "../api/collections";
import { notify } from "../../store/notificationStore";

export function SaveCollectionButton({ collection }: { collection: SavedCollection }) {
  const client = useQueryClient();
  const { data: saved = [] } = useQuery({
    queryKey: ["savedCollections"],
    queryFn: listSavedCollections,
  });
  const liked = saved.some((item) => item.id === collection.id);
  const mutation = useMutation({
    mutationFn: () => (liked ? removeSavedCollection(collection.id) : saveCollection(collection)),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["savedCollections"] });
    },
    onError: (error) =>
      notify({ kind: "error", title: "Не удалось сохранить", message: error.message }),
  });
  return (
    <button
      type="button"
      disabled={mutation.isPending}
      aria-label={liked ? "Убрать из библиотеки" : "Сохранить в библиотеку"}
      aria-pressed={liked}
      onClick={() => mutation.mutate()}
      className="flex items-center gap-2 rounded-full border border-white/15 px-4 py-2.5 text-sm text-white transition-colors hover:bg-white/10 disabled:opacity-50"
    >
      <Heart
        size={18}
        className={liked ? "text-[var(--app-accent)]" : undefined}
        fill={liked ? "currentColor" : "none"}
      />
      {liked ? "В библиотеке" : "Сохранить"}
    </button>
  );
}
