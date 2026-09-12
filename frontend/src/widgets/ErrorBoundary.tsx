import { Component, type ErrorInfo, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { AlertTriangle, RotateCcw, Home } from "lucide-react";

interface Props {
  children: ReactNode;
  /** При смене значения граница сбрасывает ошибку (напр. при навигации). */
  resetKey?: string;
  onGoHome?: () => void;
}

interface State {
  error: Error | null;
}

/**
 * Граница ошибок React. Ловит исключения рендера/жизненного цикла в поддереве
 * и показывает восстановимый экран вместо падения всего приложения.
 *
 * Зачем это критично на Android: на встроенном WebView любое необработанное
 * исключение при рендере (например, crypto.randomUUID на старом WebView или
 * ошибка в тяжёлой сетке результатов поиска) роняло React в белый экран — для
 * пользователя это выглядело как «приложение вылетело» на вкладке поиска.
 * Теперь падает только текущий экран, и его можно перезагрузить кнопкой, не
 * убивая приложение.
 */
class ErrorBoundaryInner extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidUpdate(prev: Props) {
    // Навигация на другой маршрут — сбрасываем ошибку, чтобы новый экран
    // отрисовался (кнопка «Назад» / переход по меню чинит застрявший экран).
    if (this.state.error && prev.resetKey !== this.props.resetKey) {
      this.setState({ error: null });
    }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Логируем в консоль WebView — видно в chrome://inspect при отладке.
    console.error("[ErrorBoundary] поймано исключение рендера:", error, info.componentStack);
  }

  handleRetry = () => this.setState({ error: null });

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="grid min-h-[60vh] place-items-center px-4 py-16 text-center">
        <div className="flex max-w-sm flex-col items-center gap-4">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-amber-500/15 text-amber-400">
            <AlertTriangle size={28} />
          </div>
          <div>
            <h2 className="text-lg font-bold text-white">Что-то пошло не так</h2>
            <p className="mt-1 text-sm text-slate-400">
              Этот экран не удалось отобразить. Приложение продолжает работать —
              попробуйте снова или вернитесь на главную.
            </p>
          </div>
          <div className="flex flex-wrap items-center justify-center gap-2">
            <button
              onClick={this.handleRetry}
              className="flex items-center gap-2 rounded-full bg-white/10 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-white/15"
            >
              <RotateCcw size={16} />
              Попробовать снова
            </button>
            {this.props.onGoHome ? (
              <button
                onClick={this.props.onGoHome}
                className="flex items-center gap-2 rounded-full px-4 py-2 text-sm text-slate-300 transition-colors hover:bg-white/5 hover:text-white"
              >
                <Home size={16} />
                На главную
              </button>
            ) : null}
          </div>
        </div>
      </div>
    );
  }
}

/**
 * Обёртка вокруг класса-границы: подтягивает текущий маршрут (сброс ошибки при
 * навигации) и даёт действие «На главную». Именно её используем в каркасе,
 * оборачивая <Outlet />, чтобы падение одного экрана не убивало приложение.
 */
export function RouteErrorBoundary({ children }: { children: ReactNode }) {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <ErrorBoundaryInner resetKey={location.pathname} onGoHome={() => navigate("/")}>
      {children}
    </ErrorBoundaryInner>
  );
}

export default RouteErrorBoundary;
