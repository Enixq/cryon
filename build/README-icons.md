# Иконки Cryon

`appicon.svg` — фирменная версия нового знака C с волной и кнопкой Play. Монохромный SVG интерфейса находится в `frontend/public/cryon-mark.svg`, React-версия — в `Logo.tsx`. Цвет и свечение React-логотипа задаёт `--app-accent` из палитры обложки.

Пересборка всех PNG, ICO и растров Android:

```powershell
npm ci --prefix build
node build/generate-icons.mjs
```

Android использует адаптивную иконку: отдельные фон и передний план. Android 13+ также получает `ic_launcher_monochrome.xml` для системных тематических иконок. Цвет значка в лаунчере задаёт Android; цвет внутри Cryon меняется вместе с музыкой.

Windows ICO содержит размеры 16, 24, 32, 48, 64, 128 и 256. Иконки Windows и обычный Android-значок используют постоянный фирменный градиент.
