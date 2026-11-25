import { financeCategory } from '../../functions/financeEntries';
import { formatMoney } from '../../functions/money';
import { DataTable } from '../DataTable/DataTable';
import styles from './FinanceTotals.module.css';

type FinanceTotalsProps = {
    items: readonly { label: string; value: number; emphasis?: boolean }[];
    breakdown: readonly { category: string; amountMinor: number }[];
};

export function FinanceTotals({ items, breakdown }: FinanceTotalsProps) {
    return (
        <div className={styles.totals}>
            <dl className={styles.grid} aria-label="Финансовые показатели">
                {items.map(({ label, value, emphasis }) => <div key={label} data-emphasis={emphasis ?? false}>
                    <dt>{label}</dt><dd data-negative={value < 0}>{formatMoney(value)}</dd>
                </div>)}
            </dl>
            <DataTable title="Начисления и расходы по статьям" columns={['Статья', 'Сумма']}
                rows={breakdown.map(({ category, amountMinor }, index) => ({ id: `${category}:${index}`, cells: [financeCategory(category), formatMoney(amountMinor)] }))}
                emptyMessage="Сервер не вернул начислений за выбранный период." />
        </div>
    );
}
