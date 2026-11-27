import { createEffect, sample } from 'effector';
import { userUpdated } from '@/entities/session/model';
import { api } from '@/shared/api/endpoints';

export const updateMeFx = createEffect(api.updateMe);
export const changePasswordFx = createEffect(api.changePassword);
sample({ clock: updateMeFx.doneData, target: userUpdated });
