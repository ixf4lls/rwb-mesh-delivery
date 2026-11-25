import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { flushSync } from 'react-dom';
import { analyticsApi } from '../api/analyticsApi';
import type { FinancePeriod, LedgerEntry, LedgerFilters } from '../api/analyticsApi';

type Journal = { title: string; filters: LedgerFilters };

export type FinancePdfOptions = {
    title: string;
    period: FinancePeriod;
    details?: string[];
    summary: ReactNode;
    journals: Journal[];
};

type FinancePdfDocument = Omit<FinancePdfOptions, 'journals'> & {
    journals: { title: string; entries: LedgerEntry[] }[];
};

export function useFinancePdf(options: FinancePdfOptions) {
    const [loadPage] = analyticsApi.useLazyListFinanceLedgerQuery();
    const [pdf, setPdf] = useState<FinancePdfDocument>();
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState<{ error: unknown }>();
    const active = useRef<AbortController | null>(null);

    useEffect(() => {
        const clear = () => setPdf(undefined);
        window.addEventListener('afterprint', clear);
        return () => {
            active.current?.abort();
            window.removeEventListener('afterprint', clear);
        };
    }, []);

    async function readJournal({ title, filters }: Journal, signal: AbortSignal) {
        const entries: LedgerEntry[] = [];
        const seenCursors = new Set<string>();
        let cursor: string | undefined;
        do {
            signal.throwIfAborted();
            const request = loadPage({ ...filters, cursor, limit: 200 });
            signal.addEventListener('abort', request.abort, { once: true });
            try {
                const page = await request.unwrap();
                entries.push(...page.items);
                cursor = page.nextCursor ?? undefined;
            } finally {
                signal.removeEventListener('abort', request.abort);
            }
            if (cursor && seenCursors.has(cursor)) throw new Error('Сервер повторил страницу проводок. PDF не сформирован.');
            if (cursor) seenCursors.add(cursor);
        } while (cursor);
        return { title, entries };
    }

    async function print() {
        if (active.current) return;

        const controller = new AbortController();
        active.current = controller;
        setFailure(undefined);
        setBusy(true);
        try {
            const journals: FinancePdfDocument['journals'] = [];
            for (const journal of options.journals) {
                journals.push(await readJournal(journal, controller.signal));
            }
            controller.signal.throwIfAborted();
            flushSync(() => setPdf({ ...options, journals }));
            window.print();
        } catch (error) {
            if (!controller.signal.aborted) {
                setPdf(undefined);
                setFailure({ error });
            }
        } finally {
            active.current = null;
            if (!controller.signal.aborted) setBusy(false);
        }
    }

    return { pdf, busy, failure, print };
}
