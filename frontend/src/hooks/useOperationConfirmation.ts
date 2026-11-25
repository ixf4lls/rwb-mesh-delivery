import { meshApi } from '../api/meshApi';
import type { AcceptedOperation } from '../api/meshApi';
import { operationIdFromLocation } from '../functions/operationLocation';

export function useOperationConfirmation(demoRunId?: string) {
    const [readOperation] = meshApi.useLazyGetOperationQuery();

    return async function confirm(accepted: AcceptedOperation) {
        const operationId = operationIdFromLocation(accepted.location, accepted.requestUrl, accepted.operation.id);

        for (let attempt = 0; attempt < 20; attempt += 1) {
            const operation = await readOperation({ operationId, 'X-Demo-Run-Id': demoRunId }).unwrap();
            if (operation.status === 'succeeded') return operation;
            if (operation.status === 'failed') throw { data: operation.error };

            await new Promise((resolve) => setTimeout(resolve, 1500));
        }

        throw new Error('Сервер ещё обрабатывает команду. Повторите проверку той же команды.');
    };
}
