# Иконки приложения Cryon

Источник правды — **`appicon.svg`** (тот же логотип, что в интерфейсе:
`frontend/src/shared/ui/Logo.tsx`). Растровые иконки генерируются из него.

Два файла, которые использует сборка Wails:

- `appicon.png` — исходная иконка приложения (квадрат 1024×1024);
- `windows/icon.ico` — многоразмерная иконка для `.exe` (её показывают
  Проводник, панель задач и установщик NSIS).

## Как пересобрать иконки после изменения логотипа

### Вариант 1. Node (кроссплатформенно, без установки системных программ)

```bash
cd build
npm install @resvg/resvg-js png-to-ico
node generate-icons.mjs
```

Скрипт отрендерит `appicon.png` (1024×1024) и `windows/icon.ico`
(16/24/32/48/64/128/256).

### Вариант 2. ImageMagick (одной командой, если установлен)

```bash
# PNG-иконка приложения
magick -background none build/appicon.svg -resize 1024x1024 build/appicon.png

# Многоразмерная .ico для .exe
magick -background none build/appicon.svg -define icon:auto-resize=256,128,64,48,32,24,16 build/windows/icon.ico
```

После генерации пересоберите приложение:

```bash
wails build
```

> Иконка `.exe` берётся из `windows/icon.ico` на этапе `wails build`. Если
> редактировали только `appicon.svg`, не забудьте перегенерировать растр —
> сборка не конвертирует SVG сама.
