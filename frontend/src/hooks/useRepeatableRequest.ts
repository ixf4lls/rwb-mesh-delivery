import { useRef, useState } from 'react';
import { apiErrorMessage } from '../functions/apiErrorMessage';

export function useRepeatableRequest<Payload, Result>(send: (payload: Payload, key: string) => Promise<Result>) {
    const active = useRef(false);
    const keys = useRef(new Map<string, string>());
    const [pending, setPending] = useState(false);
    const [error, setError] = useState('');

    async function submit(payload: Payload) {
        if (active.current) return;

        const signature = JSON.stringify(payload);
        const key = keys.current.get(signature) ?? crypto.randomUUID();
        keys.current.set(signature, key);
        active.current = true;
        setPending(true);
        setError('');

        try {
            const result = await send(payload, key);
            keys.current.delete(signature);
            return result;
        } catch (requestError) {
            setError(apiErrorMessage(requestError));
        } finally {
            active.current = false;
            setPending(false);
        }
    }

    return { submit, pending, error, clearError: () => setError('') };
}
