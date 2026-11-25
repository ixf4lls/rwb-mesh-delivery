import { Link } from 'react-router';
import { formatHours } from '../../functions/deliveryRoutes';
import { elapsedMinutes, financeParties } from '../../functions/financeEntries';
import { formatMoney } from '../../functions/money';
import { useOrderComparison } from '../../hooks/useOrderComparison';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { FinanceLedger } from '../FinanceLedger/FinanceLedger';
import { FinancePdfExport } from '../FinancePdfExport/FinancePdfExport';
import { FormField } from '../FormField/FormField';
import { Pagination } from '../Pagination/Pagination';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './FinanceComparison.module.css';

const orderIdPattern = '[A-Za-z0-9_\\-]+';

function comparisonRow(label: string, mesh: number | undefined, hub: number | undefined, format: (value: number) => string) {
    const diff = mesh === undefined || hub === undefined ? undefined : mesh - hub;
    return { id: label, cells: [label, mesh === undefined ? '—' : format(mesh), hub === undefined ? '—' : format(hub),
        diff === undefined ? '—' : `${diff > 0 ? '+' : ''}${format(diff)}`] };
}

export function FinanceComparison({ demoRunId, clock }: { demoRunId?: string; clock?: string }) {
    const view = useOrderComparison(demoRunId, clock);
    const selection = view.selection;
    const mesh = view.mesh.currentData;
    const hub = view.hub.currentData;
    const meshFinance = view.meshFinance.currentData;
    const hubFinance = view.hubFinance.currentData;
    const columns = ['Показатель', 'Mesh', 'Через хаб', 'Разница Mesh − хаб'];
    const ledgerKey = selection ? JSON.stringify([view.party, selection.period]) : '';
    const journals = selection && [
        { title: `Проводки Mesh · ${selection.meshId}`, filters: { ...selection.period, orderId: selection.meshId, partyType: view.party, demoRunId } },
        { title: `Проводки через хаб · ${selection.hubId}`, filters: { ...selection.period, orderId: selection.hubId, partyType: view.party, demoRunId } },
    ];
    const buyerSummary = mesh && hub && <DataTable title="Покупатель" columns={columns} rows={[
        comparisonRow('Цена доставки', mesh.buyerFeeMinor, hub.buyerFeeMinor, formatMoney),
        comparisonRow('Прогноз готовности от создания заказа', elapsedMinutes(mesh.createdAt, mesh.estimatedReadyAt),
            elapsedMinutes(hub.createdAt, hub.estimatedReadyAt), formatHours),
        comparisonRow('Фактическое время до выдачи', elapsedMinutes(mesh.createdAt, mesh.issuedAt),
            elapsedMinutes(hub.createdAt, hub.issuedAt), formatHours),
    ]} emptyMessage="Показатели недоступны." />;
    const sellerSummary = (meshFinance || hubFinance) && <>
        <DataTable title="Продавец по заказам" columns={columns} rows={[
            comparisonRow('Выручка', meshFinance?.salesMinor, hubFinance?.salesMinor, formatMoney),
            comparisonRow('Удержания', meshFinance?.deductionsMinor, hubFinance?.deductionsMinor, formatMoney),
            comparisonRow('Начислено', meshFinance?.accrualMinor, hubFinance?.accrualMinor, formatMoney),
            comparisonRow('Себестоимость', meshFinance?.goodsCostMinor, hubFinance?.goodsCostMinor, formatMoney),
            comparisonRow('Прибыль', meshFinance?.profitMinor, hubFinance?.profitMinor, formatMoney),
        ]} emptyMessage="Расчёты продавца ещё недоступны." />
        {(meshFinance?.settlementStatus !== 'posted' || hubFinance?.settlementStatus !== 'posted')
            && <p className={styles.hint}>По одному или обоим заказам расчёт ещё не завершён. Показаны доступные суммы.</p>}
    </>;
    const pdfReady = Boolean(meshFinance && hubFinance) && [view.mesh, view.hub, view.meshFinance, view.hubFinance]
        .every(({ isFetching, isError }) => !isFetching && !isError);

    return (
        <div className={styles.workspace}>
            <form className={styles.filter} onSubmit={view.compare}>
                <FormField label="Заказ Mesh"><input name="meshId" list={view.orderOptionsId} maxLength={128} pattern={orderIdPattern} required /></FormField>
                <FormField label="Заказ через хаб"><input name="hubId" list={view.orderOptionsId} maxLength={128} pattern={orderIdPattern} required /></FormField>
                <datalist id={view.orderOptionsId}>
                    {view.choices.currentData?.items.map((order) => <option key={order.id} value={order.id}>{order.pickupPointId}</option>)}
                </datalist>
                <FormField label="Начало периода"><input name="start" type="date" defaultValue={view.dates.start} required /></FormField>
                <FormField label="Конец периода"><input name="end" type="date" defaultValue={view.dates.end} required /></FormField>
                <Button type="submit">Сравнить заказы</Button>
            </form>
            <p className={styles.hint}>Выберите заказы с одинаковой корзиной и ПВЗ выдачи. Период ограничивает журнал проводок, обе даты включены по UTC.</p>
            <Pagination cursor={view.page.cursor} nextCursor={view.choices.currentData?.nextCursor} busy={view.choices.isFetching}
                onPrevious={view.page.previous} onNext={view.page.next} />
            {view.choices.isError && <QueryNotice error={view.choices.error} onRetry={view.choices.refetch} />}
            {view.validation && <p className={styles.error} role="alert">{view.validation}</p>}
            {view.mesh.isError && <QueryNotice error={view.mesh.error} onRetry={view.mesh.refetch} />}
            {view.hub.isError && <QueryNotice error={view.hub.error} onRetry={view.hub.refetch} />}
            {selection && !mesh && !hub && (view.mesh.isFetching || view.hub.isFetching) && <QueryNotice message="Загружаем заказы…" />}
            {mesh && hub && !view.comparable && <p className={styles.error} role="alert">Заказы отличаются товарами, количеством или ПВЗ выдачи.</p>}
            {selection && mesh && hub && journals && view.comparable && <>
                <h2 className={styles.title}>Mesh {mesh.id} · через хаб {hub.id}</h2>
                {buyerSummary}
                <div className={styles.selector}>
                    <FormField label="Продавец в корзине">
                        <select value={view.sellerId ?? ''} onChange={(event) => view.selectSeller(event.currentTarget.value)}>
                            {view.sellers.map((id) => <option key={id} value={id}>{id}</option>)}
                        </select>
                    </FormField>
                </div>
                {view.meshFinance.isError && <QueryNotice error={view.meshFinance.error} onRetry={view.meshFinance.refetch} />}
                {view.hubFinance.isError && <QueryNotice error={view.hubFinance.error} onRetry={view.hubFinance.refetch} />}
                {(view.meshFinance.isFetching || view.hubFinance.isFetching) && !meshFinance && !hubFinance && <QueryNotice message="Загружаем расчёты продавца…" />}
                {sellerSummary}
                <div className={styles.selector}>
                    <FormField label="Чьи проводки сравнить">
                        <select value={view.party} onChange={(event) => view.setParty(event.currentTarget.value === 'marketplace' ? 'marketplace' : 'pvz')}>
                            <option value="pvz">ПВЗ</option><option value="marketplace">Маркетплейс</option>
                        </select>
                    </FormField>
                </div>
                <FinancePdfExport key={JSON.stringify([selection, view.party, view.sellerId])} title="Сравнение Mesh и доставки через хаб"
                    period={selection.period} summary={<>{buyerSummary}{sellerSummary}</>} enabled={pdfReady} journals={journals}
                    details={[`Mesh ${mesh.id} · через хаб ${hub.id}`, `ПВЗ выдачи ${mesh.pickupPointId}`,
                        `Продавец ${view.sellerId ?? '—'}`, `Проводки · ${financeParties[view.party]}`]} />
                <div className={styles.ledgers}>
                    {journals.map(({ title, filters }) => <FinanceLedger key={`${title}:${ledgerKey}`} compact title={title} filters={filters} />)}
                </div>
                <p className={styles.hint}>Показаны проведённые суммы каждого участника. Прогноз затрат и сроков до создания заказа доступен в <Link to="/delivery-model">модели доставки</Link>.</p>
            </>}
        </div>
    );
}
