import { expect, it, vi, beforeEach } from "vitest";
import { attachHls } from "./hlsPlayback";

const fake = vi.hoisted(() => ({
  callbacks: {} as Record<string, (...args: unknown[]) => void>,
  attachMedia: vi.fn(),
  loadSource: vi.fn(),
  destroy: vi.fn(),
}));
vi.mock("hls.js", () => ({
  default: class {
    static isSupported = () => true;
    static Events = { MEDIA_ATTACHED: "attached", ERROR: "error" };
    static ErrorTypes = { NETWORK_ERROR: "network" };
    on(event: string, callback: (...args: unknown[]) => void) {
      fake.callbacks[event] = callback;
    }
    attachMedia = fake.attachMedia;
    loadSource = fake.loadSource;
    destroy = fake.destroy;
  },
}));
beforeEach(() => {
  vi.clearAllMocks();
  fake.callbacks = {};
});

it("loads HLS into the existing audio element and disposes it on track change", () => {
  const audio = document.createElement("audio");
  const failed = vi.fn();
  const dispose = attachHls(audio, "http://localhost/v1/vk/stream?id=web_1_2", failed);
  expect(fake.attachMedia).toHaveBeenCalledWith(audio);
  fake.callbacks.attached();
  expect(fake.loadSource).toHaveBeenCalledWith("http://localhost/v1/vk/stream?id=web_1_2");
  fake.callbacks.error("error", { fatal: false });
  expect(failed).not.toHaveBeenCalled();
  fake.callbacks.error("error", { fatal: true, type: "network" });
  expect(failed).toHaveBeenCalledWith("ВК: не удалось загрузить сегмент аудио.");
  dispose?.();
  expect(fake.destroy).toHaveBeenCalledOnce();
});
