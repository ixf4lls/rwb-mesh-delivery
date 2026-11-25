import type { SellerEconomics } from '../../api/sellerApi';
import { formatMoney } from '../../functions/money';
import { DataTable } from '../DataTable/DataTable';
import styles from './SellerAmounts.module.css';

type Amounts = Pick<SellerEconomics, 'salesMinor' | 'deductionsMinor' | 'accrualMinor' | 'goodsCostMinor' | 'profitMinor' | 'breakdown'>;

const amounts = [
    ['Выручка', 'salesMinor'], ['Удержания', 'deductionsMinor'], ['Начислено', 'accrualMinor'],
    ['Себестоимость', 'goodsCostMinor'], ['Прибыль', 'profitMinor'],
] as const;

const categories: Record<string, string> = {
    sale: 'Продажа', seller_accrual: 'Начисление продавцу', storage: 'Хранение', seller_deduction: 'Удержание',
    handling: 'Обработка', return: 'Возврат', goods_cost: 'Себестоимость',
};

export function SellerAmounts({ data }: { data: Amounts }) {
    const rows = data.breakdown.map(({ category, amountMinor }, index) => ({
        id: `${category}:${index}`, cells: [categories[category] ?? category, formatMoney(amountMinor)],
    }));

    return (
        <div className={styles.amounts}>
            <dl className={styles.metrics} aria-label="Финансовые показатели">
                {amounts.map(([label, key]) => (
                    <div key={key} className={key === 'profitMinor' ? styles.profit : undefined} data-negative={data[key] < 0}>
                        <dt>{label}</dt>
                        <dd>{formatMoney(data[key])}</dd>
                    </div>
                ))}
            </dl>
            <DataTable title="Детализация" columns={['Статья', 'Сумма']} rows={rows} emptyMessage="За этот период нет проведённых операций." />
        </div>
    );
}
