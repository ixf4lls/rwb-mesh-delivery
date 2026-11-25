import type { Order } from '../../api/pvzApi';
import { formatDate } from '../../functions/formatDate';
import { formatMoney } from '../../functions/money';
import { useDeliveryFinancials } from '../../hooks/useDeliveryFinancials';
import { DataTable } from '../DataTable/DataTable';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import { SellerAmounts } from '../SellerAmounts/SellerAmounts';
import styles from './DeliveryFinancials.module.css';

export function DeliveryFinancials({ order, clock, demoRunId }: { order: Order; clock?: string; demoRunId?: string }) {
    const view = useDeliveryFinancials(order, demoRunId, clock);
    const point = view.point.currentData;
    const market = view.market.currentData;
    const seller = view.seller.currentData;
    const rows = [
        ...(point ? [{ id: 'point', cells: ['ПВЗ выдачи', formatMoney(point.rewardMinor), formatMoney(point.operatingCostMinor),
            formatMoney(point.deductionsMinor), formatMoney(point.profitMinor)] }] : []),
        ...(market ? [{ id: 'market', cells: ['Маркетплейс', formatMoney(market.revenueMinor), formatMoney(market.expenseMinor),
            '—', formatMoney(market.profitMinor)] }] : []),
    ];

    return (
        <details className={styles.panel} onToggle={(event) => view.setOpen(event.currentTarget.open)}>
            <summary>Финансы участников</summary>
            <div className={styles.body}>
                <p>Покупатель по заказу {order.id} · доставка {formatMoney(order.buyerFeeMinor)} · ожидаемая готовность {formatDate(order.estimatedReadyAt)}.</p>
                <FormField label="Продавец в заказе">
                    <select value={view.sellerId ?? ''} onChange={(event) => view.selectSeller(event.currentTarget.value)}>
                        {view.sellers.map((id) => <option key={id} value={id}>{id}</option>)}
                    </select>
                </FormField>
                {view.seller.isError && <QueryNotice error={view.seller.error} onRetry={view.seller.refetch} />}
                {seller && <SellerAmounts data={seller} />}
                <p>Сводные отчёты ПВЗ и маркетплейса за {view.dates.start} — {view.dates.end}, UTC.</p>
                {view.point.isError && <QueryNotice error={view.point.error} onRetry={view.point.refetch} />}
                {view.market.isError && <QueryNotice error={view.market.error} onRetry={view.market.refetch} />}
                {rows.length > 0 && <DataTable title="Проведённые суммы за период"
                    columns={['Участник', 'Доход или вознаграждение', 'Расходы', 'Удержания', 'Финансовый результат']}
                    rows={rows} emptyMessage="Отчёт недоступен." />}
            </div>
        </details>
    );
}
