import { useAnalyticsWorkspace } from '../../hooks/useAnalyticsWorkspace';
import { Button } from '../Button/Button';
import { FinanceComparison } from '../FinanceComparison/FinanceComparison';
import { FinanceReport } from '../FinanceReport/FinanceReport';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './AnalyticsWorkspace.module.css';

export default function AnalyticsWorkspace() {
    const view = useAnalyticsWorkspace();
    const scope = { demoRunId: view.demoRunId, clock: view.demo.currentData?.clock };

    return (
        <div className={styles.workspace}>
            {view.demo.isError && <QueryNotice error={view.demo.error} onRetry={view.demo.refetch} />}
            {view.demo.isLoading && <QueryNotice message="Загружаем демонабор…" />}
            {view.scopeReady && <>
                <div className={styles.tabs} role="group" aria-label="Разделы финансовой аналитики">
                    <Button variant={view.mode === 'report' ? 'primary' : 'secondary'} aria-pressed={view.mode === 'report'}
                        onClick={() => view.setMode('report')}>Сводка и проводки</Button>
                    <Button variant={view.mode === 'comparison' ? 'primary' : 'secondary'} aria-pressed={view.mode === 'comparison'}
                        onClick={() => view.setMode('comparison')}>Сравнение заказов</Button>
                </div>
                {view.mode === 'report' ? <FinanceReport key={view.demoRunId} {...scope} /> : <FinanceComparison key={view.demoRunId} {...scope} />}
            </>}
        </div>
    );
}
