import type { LedgerEntry, LedgerFilters } from '../../api/analyticsApi';
import { financeCategory, financeParties } from '../../functions/financeEntries';
import { formatDate } from '../../functions/formatDate';
import { formatMoney } from '../../functions/money';
import { useFinanceLedger } from '../../hooks/useFinanceLedger';
import { DataTable } from '../DataTable/DataTable';
import { Pagination } from '../Pagination/Pagination';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './FinanceLedger.module.css';

const fullColumns = ['Участник', 'Статья', 'Сумма', 'Время проводки', 'Источник', 'Тариф'];
const compactColumns = ['Статья и участник', 'Сумма', 'Проводка'];

export function FinanceLedgerTable({ entries, title, compact = false }: { entries: LedgerEntry[]; title: string; compact?: boolean }) {
    const rows = entries.map((entry) => {
        const amount = <strong className={entry.amountMinor < 0 ? styles.negative : undefined}>{formatMoney(entry.amountMinor)}</strong>;
        const party = `${financeParties[entry.partyType]} · ${entry.partyId}`;
        const source = <div>
            {entry.id}<small>Событие {entry.sourceEventId}</small>
            {entry.orderId && <small>Заказ {entry.orderId}</small>}
            {entry.batchId && <small>Партия {entry.batchId}</small>}
            {entry.returnId && <small>Возврат {entry.returnId}</small>}
            {entry.originalEntryId && <small>Сторно {entry.originalEntryId}</small>}
        </div>;
        return { id: entry.id, cells: compact ? [<div>{financeCategory(entry.category)}<small>{party}</small></div>, amount, source] : [
            party, financeCategory(entry.category), amount, formatDate(entry.bookedAt), source, entry.tariffVersionId,
        ] };
    });

    return <div className={styles.ledger}>
        <DataTable title={title} columns={compact ? compactColumns : fullColumns} rows={rows} emptyMessage="За выбранный период проводок нет." />
    </div>;
}

export function FinanceLedger({ filters, title, compact = false }: { filters: LedgerFilters; title: string; compact?: boolean }) {
    const { page, ledger } = useFinanceLedger(filters);

    return (
        <div className={styles.ledger}>
            {ledger.isError && <QueryNotice error={ledger.error} onRetry={ledger.refetch} />}
            {ledger.isFetching && !ledger.currentData && <QueryNotice message="Загружаем проводки…" />}
            {ledger.currentData && <>
                <FinanceLedgerTable entries={ledger.currentData.items} title={title} compact={compact} />
                <Pagination cursor={page.cursor} nextCursor={ledger.currentData.nextCursor} busy={ledger.isFetching}
                    onPrevious={page.previous} onNext={page.next} />
            </>}
        </div>
    );
}
