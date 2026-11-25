import { useDispatch } from 'react-redux';
import { meshApi } from '../api/meshApi';
import { pvzApi } from '../api/pvzApi';
import type { OrderAction } from '../functions/pvzOrders';
import { useOperationConfirmation } from './useOperationConfirmation';
import { useRepeatableRequest } from './useRepeatableRequest';

const orderTags = ['Orders', 'OrderRoute', 'Batches', 'Capacity', 'Economics'] as const;

export function useOrderCommand(demoRunId?: string) {
    const dispatch = useDispatch();
    const [recordEvent] = pvzApi.useRecordOrderEventMutation();
    const confirmOperation = useOperationConfirmation(demoRunId);

    return useRepeatableRequest(async ({ orderId, body }: OrderAction, key) => {
        try {
            const accepted = await recordEvent({ orderId, body, key, demoRunId }).unwrap();
            return await confirmOperation(accepted);
        } finally {
            dispatch(meshApi.util.invalidateTags([...orderTags]));
        }
    });
}
