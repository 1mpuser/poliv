import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react';

type Undo = () => void | Promise<void>;
type ShowToast = (message: string, undo?: Undo) => void;

const ToastContext = createContext<ShowToast>(() => {});
export const useToast = () => useContext(ToastContext);

/** Тост из макета: сообщение + «Отменить», прячется через 5 секунд */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<{ message: string; undo?: Undo; shown: boolean }>({
    message: '',
    shown: false,
  });
  const timer = useRef<number | undefined>(undefined);

  const show = useCallback<ShowToast>((message, undo) => {
    window.clearTimeout(timer.current);
    setState({ message, undo, shown: true });
    timer.current = window.setTimeout(() => setState((s) => ({ ...s, shown: false })), 5000);
  }, []);

  const onUndo = () => {
    window.clearTimeout(timer.current);
    setState((s) => ({ ...s, shown: false }));
    state.undo?.();
  };

  return (
    <ToastContext.Provider value={show}>
      {children}
      <div className={`toast${state.shown ? ' is-shown' : ''}`} role="status" aria-live="polite">
        <span className="toast__text">{state.message}</span>
        <button className="toast__undo" type="button" hidden={!state.undo} onClick={onUndo}>
          Отменить
        </button>
      </div>
    </ToastContext.Provider>
  );
}
