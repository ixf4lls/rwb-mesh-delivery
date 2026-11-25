import { useDispatch } from 'react-redux';
import { meshApi } from '../api/meshApi';
import { pvzApi } from '../api/pvzApi';
import { operationIdFromLocation } from '../functions/operationLocation';
import type { OrderAction } from '../functions/pvzOrders';
import { useRepeatableRequest } from './useRepeatableRequest';

const orderTags = ['Orders', 'OrderRoute', 'Batches', 'Capacity'] as const;

export function useOrderCommand(demoRunId?: string) {
    const dispatch = useDispatch();
    const [recordEvent] = pvzApi.useRecordOrderEventMutation();
    const [readOperation] = meshApi.useLazyGetOperationQuery();

    return useRepeatableRequest(async ({ orderId, body }: OrderAction, key) => {
        try {
            const accepted = await recordEvent({ orderId, body, key, demoRunId }).unwrap();
            const operationId = operationIdFromLocation(accepted.location, accepted.requestUrl, accepted.operation.id);

            for (let attempt = 0; attempt < 20; attempt += 1) {
                const operation = await readOperation({ operationId, 'X-Demo-Run-Id': demoRunId }).unwrap();
                if (operation.status === 'succeeded') return operation;
                if (operation.status === 'failed') throw { data: operation.error };

                await new Promise((resolve) => setTimeout(resolve, 1500));
            }

            throw new Error('Сервер ещё обрабатывает команду. Повторите проверку той же команды.');
        } finally {
            dispatch(meshApi.util.invalidateTags([...orderTags]));
        }
    });
}
