import { useState } from 'react';
import type { FormEvent } from 'react';
import type { StorageCase } from '../api/exceptionApi';
import { modelTimeFromInput, physicalSegments } from '../functions/exceptionRoutes';
import { useExceptionCommand } from './useExceptionCommand';

type StorageForm = 'decision' | 'handed_over' | 'arrived';

export function useStorageWorkflow(cases: readonly StorageCase[], demoRunId?: string, clock?: string) {
    const [selection, setSelection] = useState<{ id: string; version: number; form: StorageForm }>();
    const [decision, setDecision] = useState<'keep' | 'transfer_to_hub'>('keep');
    const [validation, setValidation] = useState('');
    const [message, setMessage] = useState('');
    const item = cases.find(({ id }) => id === selection?.id);
    const segments = item ? physicalSegments(item) : [];
    const command = useExceptionCommand(demoRunId);

    function open(current: StorageCase, form: StorageForm) {
        setSelection({ id: current.id, version: current.version, form });
        setDecision(current.recommendation);
        setValidation('');
        setMessage('');
        command.clearError();
    }

    async function submit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (!selection || !item) return;
        const fields = new FormData(event.currentTarget);
        const expectedVersion = selection.version;
        setValidation('');

        if (selection.form === 'decision') {
            const reason = String(fields.get('reason') ?? '').trim();
            const reference = clock ?? new Date().toISOString();
            const recheckAt = decision === 'keep' ? modelTimeFromInput(String(fields.get('recheckAt') ?? ''), reference) : undefined;
            if (!reason || (decision === 'keep' && !recheckAt)) {
                setValidation('Укажите причину и, если партия остаётся, дату проверки позже текущего времени.');
                return;
            }

            const result = await command.submit({ kind: 'decideStorage', id: selection.id,
                body: { action: decision, reason, expectedVersion, recheckAt } });
            if (result) {
                setSelection(undefined);
                setMessage('Решение по хранению подтверждено.');
            }
            return;
        }

        const segmentId = String(fields.get('segmentId') ?? '');
        if (!segments.some(({ id }) => id === segmentId)) {
            setValidation('Выберите физический сегмент вывоза.');
            return;
        }

        const result = await command.submit({ kind: 'transferStorage', id: selection.id, body: { type: selection.form, segmentId, expectedVersion } });
        if (result) {
            setSelection(undefined);
            setMessage('Этап вывоза подтверждён.');
        }
    }

    return { selection, decision, setDecision, item, segments, open, close: () => setSelection(undefined), submit, command, validation, message };
}
