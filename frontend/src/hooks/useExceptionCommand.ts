import { useDispatch } from 'react-redux';
import { commandRoots, exceptionApi } from '../api/exceptionApi';
import type { ExceptionCommand } from '../api/exceptionApi';
import { meshApi } from '../api/meshApi';
import { useOperationConfirmation } from './useOperationConfirmation';
import { useRepeatableRequest } from './useRepeatableRequest';

const tags = ['Returns', 'Incidents', 'StorageCases', 'Notices', 'Orders', 'OrderRoute', 'Capacity', 'Batches', 'Economics'] as const;

export function useExceptionCommand(demoRunId?: string) {
    const dispatch = useDispatch();
    const [post] = exceptionApi.useRunExceptionCommandMutation();
    const confirm = useOperationConfirmation(demoRunId);

    return useRepeatableRequest(async (command: ExceptionCommand, key) => {
        try {
            return await confirm(await post({ command, key, demoRunId }).unwrap(), commandRoots[command.kind]);
        } finally {
            dispatch(meshApi.util.invalidateTags(command.kind === 'advanceClock' ? [...tags, 'DemoContext'] : [...tags]));
        }
    });
}
