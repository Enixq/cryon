import React, { useEffect } from "react";
import { createRoot } from "react-dom/client";
import { useAudioEngine } from "../../frontend/src/store/audioEngine";
import { usePlayerStore } from "../../frontend/src/store/playerStore";
import { startDeviceSync } from "../../frontend/src/shared/lib/useDeviceSync";
import type { Track } from "../../frontend/src/shared/types";

const testWindow = window as typeof window & {
  __CRYON_TEST_QUEUE__: Track[];
  testPlayer: typeof usePlayerStore;
  testCommands: Record<string, number>;
};
const native = window.CryonAndroid!;
testWindow.testCommands = { play: 0, pause: 0, resume: 0 };
window.CryonAndroid = {
  nativePlay: (...args) => { testWindow.testCommands.play++; native.nativePlay!(...args); },
  nativePause: (id) => { testWindow.testCommands.pause++; native.nativePause!(id); },
  nativeResume: (id) => { testWindow.testCommands.resume++; native.nativeResume!(id); },
  nativeStop: (id) => native.nativeStop!(id),
  nativeGetState: () => native.nativeGetState!(),
  nativeSetQueue: (json) => native.nativeSetQueue!(json),
  nativeConfigureSync: (json) => native.nativeConfigureSync!(json),
  nativeSeek: (position) => native.nativeSeek!(position),
  nativeSetVolume: (volume) => native.nativeSetVolume!(volume),
};
usePlayerStore.setState({ queue: testWindow.__CRYON_TEST_QUEUE__, currentIndex: 0, isPlaying: true, radio: false });
testWindow.testPlayer = usePlayerStore;
function Harness() {
  useAudioEngine();
  useEffect(startDeviceSync, []);
  const position = usePlayerStore((state) => state.progress);
  return <div>Position: {position}</div>;
}
createRoot(document.getElementById("root")!).render(<Harness />);
