import { useState } from 'react';
import { useDemoScope } from './useDemoScope';

export function useAnalyticsWorkspace() {
    const [mode, setMode] = useState<'report' | 'comparison'>('report');

    return { mode, setMode, ...useDemoScope() };
}
