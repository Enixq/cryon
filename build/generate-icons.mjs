// Генерация растровых иконок приложения из build/appicon.svg.
//
// Зачем: Wails собирает Windows-иконку .exe из build/windows/icon.ico, а
// build/appicon.png используется как исходник иконки приложения. Оба —
// растровые, поэтому их нельзя держать «в векторе»; этот скрипт пересобирает
// их из единственного источника правды — appicon.svg (он совпадает с логотипом
// в интерфейсе, frontend/src/shared/ui/Logo.tsx).
//
// Запуск (разово, когда логотип изменился):
//   cd build
//   npm install @resvg/resvg-js png-to-ico
//   node generate-icons.mjs
//
// Альтернатива без Node — одной командой ImageMagick (см. README-icons.md).

import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { Resvg } from "@resvg/resvg-js";
import pngToIco from "png-to-ico";

const here = dirname(fileURLToPath(import.meta.url));
const svg = readFileSync(join(here, "appicon.svg"));

/** Рендерит SVG в PNG-буфер заданного размера (квадрат). */
function renderPng(size) {
  const resvg = new Resvg(svg, {
    fitTo: { mode: "width", value: size },
    background: "rgba(0,0,0,0)",
  });
  return resvg.render().asPng();
}

// Основная иконка приложения: 1024×1024.
const appicon = renderPng(1024);
writeFileSync(join(here, "appicon.png"), appicon);
console.log("✓ build/appicon.png (1024×1024)");

// Windows .ico: набор размеров, которые Explorer/панель задач выбирают по DPI.
const icoSizes = [16, 24, 32, 48, 64, 128, 256];
const icoPngs = icoSizes.map(renderPng);
const ico = await pngToIco(icoPngs);
writeFileSync(join(here, "windows", "icon.ico"), ico);
console.log(`✓ build/windows/icon.ico (${icoSizes.join(", ")})`);

console.log("Готово. Пересоберите приложение: wails build.");
