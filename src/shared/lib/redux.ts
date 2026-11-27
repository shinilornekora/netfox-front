// Типизированные хуки react-redux. Типы стора берутся из app/store
// (импорт только типов — слой shared от app в рантайме не зависит).
import { useDispatch, useSelector } from 'react-redux';
import type { AppDispatch, RootState } from '@/app/store';

export const useAppDispatch = useDispatch.withTypes<AppDispatch>();
export const useAppSelector = useSelector.withTypes<RootState>();
export type { AppDispatch, RootState };
