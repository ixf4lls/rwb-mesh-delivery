import { useState } from 'react';
import type { FormEvent } from 'react';
import type { DemoRun } from '../api/deliveryApi';
import { modelTimeFromInput } from '../functions/exceptionRoutes';
import { useExceptionCommand } from './useExceptionCommand';

export function useDemoClock(run: DemoRun) {
    const [validation, setValidation] = useState('');
    const [advanced, setAdvanced] = useState(false);
    const command = useExceptionCommand(run.id);

    async function advance(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const form = event.currentTarget;
        const fields = new FormData(form);
        const to = modelTimeFromInput(String(fields.get('to') ?? ''), run.clock);
        setAdvanced(false);
        if (!to) {
            setValidation('Новое модельное время должно быть позже текущего.');
            return;
        }

        setValidation('');
        const result = await command.submit({ kind: 'advanceClock', body: { to } });
        if (result) {
            form.reset();
            setAdvanced(true);
        }
    }

    return { advance, validation, advanced, command };
}
