// Side-effect модуль: ставит HTTP-мост Android/LAN как можно раньше.
//
// Импортируется ПЕРВЫМ в main.tsx, потому что некоторые модули (например
// store/playerStore) читают isWailsRuntime() уже на этапе загрузки. ES-импорты
// вычисляются в порядке объявления, поэтому этот модуль отрабатывает до импорта
// App и его зависимостей — и window.go/window.runtime уже готовы. На десктопе и
// без заданного адреса сервера — no-op.
import { installHttpBridge } from "./httpBridge";

installHttpBridge();
