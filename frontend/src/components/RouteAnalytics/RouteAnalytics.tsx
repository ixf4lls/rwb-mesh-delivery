import type { LogisticsNode, Quote, Shipment } from '../../api/deliveryApi';
import { costNames, formatHours, optionDifference, pointName, segmentDistance, segmentDuration, segmentNames } from '../../functions/deliveryRoutes';
import { formatMoney } from '../../functions/money';
import { DataTable } from '../DataTable/DataTable';
import styles from './RouteAnalytics.module.css';

type RouteAnalyticsProps = { quote?: Quote; shipments?: readonly Shipment[]; points: ReadonlyMap<string, LogisticsNode> };

const comparison = [
    { key: 'estimatedCostMinor', title: 'Логистические затраты', format: formatMoney },
    { key: 'buyerFeeMinor', title: 'Цена доставки для покупателя', format: formatMoney },
    { key: 'etaMinutes', title: 'Срок доставки', format: formatHours },
] as const;

const percent = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1, signDisplay: 'exceptZero' });

export function RouteAnalytics({ quote, shipments, points }: RouteAnalyticsProps) {
    const mesh = quote?.options.find(({ policy }) => policy === 'mesh_only');
    const hub = quote?.options.find(({ policy }) => policy === 'hub_only');
    const comparisons = comparison.map(({ key, title, format }) => {
        const delta = optionDifference(mesh, hub, key);
        const meshValue = mesh?.[key];
        const hubValue = hub?.[key];
        return { id: key, cells: [title, meshValue === undefined ? '—' : format(meshValue), hubValue === undefined ? '—' : format(hubValue),
            delta ? <strong className={delta.amount < 0 ? styles.saving : delta.amount > 0 ? styles.extra : undefined}>
                {delta.amount > 0 ? '+' : ''}{format(delta.amount)}{delta.percent === undefined ? '' : ` (${percent.format(delta.percent)} %)`}
            </strong> : '—'] };
    });
    const segments = shipments?.flatMap((shipment) => shipment.segments.map((segment, index) => ({
        id: `${shipment.id}:${index}`,
        cells: [segmentNames[segment.type], `${pointName(points, segment.fromNodeId)} → ${pointName(points, segment.toNodeId)}`,
            segmentDistance(segment), `${segmentDuration(segment)} мин`, `${segment.waitMinutes} мин`, formatMoney(segment.estimatedCostMinor),
            segment.costItems.map(({ category, amountMinor }) => `${costNames[category]} ${formatMoney(amountMinor)}`).join(' · ') || '—',
            segment.mode === 'simulated' ? 'Симуляция' : 'Физический'],
    }))) ?? [];

    return (
        <div className={styles.analysis}>
            {quote && <>
                <DataTable title="Сравнение одной корзины" columns={['Показатель', 'Mesh', 'Через хаб', 'Разница Mesh − хаб']}
                    rows={comparisons} emptyMessage="Варианты для сравнения отсутствуют." />
            </>}
            {segments.length > 0 && <DataTable title="Сегменты выбранного маршрута"
                columns={['Этап', 'Откуда → куда', 'Расстояние', 'В пути', 'Ожидание', 'Затраты', 'Из чего складываются', 'Режим']}
                rows={segments} emptyMessage="Сегментов нет." />}
        </div>
    );
}
