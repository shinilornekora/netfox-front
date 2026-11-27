import { allSettled, type Scope } from 'effector';
import type { Gate } from 'effector-react';

/** Открыть gate без пропсов так же, как useGate в компоненте (props = {}). */
export function openGate(gate: Gate<void>, scope: Scope) {
  return allSettled(gate.open, { scope, params: {} as unknown as void });
}
