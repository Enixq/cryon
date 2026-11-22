import { create } from "zustand";
export type DownloadProgress = {
  id: string;
  title: string;
  progress: number;
  message: string;
  state: "loading" | "saved" | "error";
};
export const useDownloadStore = create<{
  items: DownloadProgress[];
  update: (item: DownloadProgress) => void;
  dismiss: (id: string) => void;
}>((set) => ({
  items: [],
  update: (item) => set((s) => ({ items: [...s.items.filter((row) => row.id !== item.id), item] })),
  dismiss: (id) => set((s) => ({ items: s.items.filter((row) => row.id !== id) })),
}));
