/// <reference types="vite/client" />
declare module "*.css";

// Минимальная типизация Wails runtime, доступного в собранном .exe.
// В браузере при разработке методы могут отсутствовать — вызываем через optional chaining.
declare global {
  interface Window {
    runtime?: {
      WindowMinimise?: () => void;
      WindowToggleMaximise?: () => void;
      WindowMaximise?: () => void;
      WindowUnmaximise?: () => void;
      Quit?: () => void;
    };
  }
}

export {};
