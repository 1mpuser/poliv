import { createContext, useContext } from 'react';
import type { Me } from './types';

/** Текущая учётка; внутри приложения после входа всегда задана */
export const MeContext = createContext<Me | null>(null);
export const useMe = () => useContext(MeContext)!;
