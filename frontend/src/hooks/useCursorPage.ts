import { useState } from 'react';

export function useCursorPage() {
    const [history, setHistory] = useState<string[]>([]);

    return {
        cursor: history.at(-1),
        previous: () => setHistory((pages) => pages.slice(0, -1)),
        next: (cursor: string) => setHistory((pages) => [...pages, cursor]),
        reset: () => setHistory([]),
    };
}
