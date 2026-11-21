let stopOutput: () => Promise<void> = async () => {};

export function registerLocalPlaybackStop(stop: () => Promise<void>): () => void {
  stopOutput = stop;
  return () => {
    if (stopOutput === stop) stopOutput = async () => {};
  };
}

export function stopLocalPlayback(): Promise<void> {
  return stopOutput();
}
