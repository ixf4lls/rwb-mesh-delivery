import { createPortal } from 'react-dom';
import { formatFinancePeriod } from '../../functions/financeEntries';
import { useFinancePdf } from '../../hooks/useFinancePdf';
import type { FinancePdfOptions } from '../../hooks/useFinancePdf';
import { Button } from '../Button/Button';
import { FinanceLedgerTable } from '../FinanceLedger/FinanceLedger';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './FinancePdfExport.module.css';

export function FinancePdfExport({ enabled, ...options }: FinancePdfOptions & { enabled: boolean }) {
    const { pdf, busy, failure, print } = useFinancePdf(options);

    return (
        <div>
            <Button variant="secondary" disabled={!enabled || busy} aria-busy={busy} onClick={() => void print()}>
                {busy ? 'Готовим PDF…' : 'Сохранить в PDF'}
            </Button>
            {failure && <QueryNotice error={failure.error} />}
            {pdf && createPortal(
                <article className={styles.document} data-finance-pdf>
                    <header className={styles.header}>
                        <p>RWB Mesh Delivery</p>
                        <h1>{pdf.title}</h1>
                        <p>Период проводок {formatFinancePeriod(pdf.period)} (UTC)</p>
                        {pdf.details?.map((detail) => <p key={detail}>{detail}</p>)}
                    </header>
                    <div className={styles.summary}>{pdf.summary}</div>
                    {pdf.journals.map(({ title, entries }) => <FinanceLedgerTable key={title} title={title} entries={entries} />)}
                </article>,
                document.body,
            )}
        </div>
    );
}
