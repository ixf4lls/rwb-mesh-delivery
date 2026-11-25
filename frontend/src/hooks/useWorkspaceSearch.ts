import { useState } from 'react';
import type { FocusEvent, FormEvent, KeyboardEvent } from 'react';
import { useNavigate } from 'react-router';
import { workspaceRoutes } from '../app/workspaceRoutes';

export function useWorkspaceSearch() {
    const [query, setQuery] = useState('');
    const [hasFocus, setHasFocus] = useState(false);
    const navigate = useNavigate();
    const normalizedQuery = query.trim().toLocaleLowerCase('ru');
    const results = workspaceRoutes.filter(({ title, description }) => (
        `${title} ${description}`.toLocaleLowerCase('ru').includes(normalizedQuery)
    ));

    function goToFirstResult(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();

        if (normalizedQuery && results[0]) {
            navigate(results[0].path);
            setQuery('');
        }
    }

    function handleBlur(event: FocusEvent<HTMLFormElement>) {
        if (!event.currentTarget.contains(event.relatedTarget)) setHasFocus(false);
    }

    function handleEscape(event: KeyboardEvent<HTMLFormElement>) {
        if (event.key === 'Escape') setQuery('');
    }

    return {
        query,
        results,
        showResults: hasFocus && normalizedQuery.length > 0,
        setQuery,
        handleFocus: () => setHasFocus(true),
        handleBlur,
        handleEscape,
        goToFirstResult,
        clearSearch: () => setQuery(''),
    };
}
