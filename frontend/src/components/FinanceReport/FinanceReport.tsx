import type { FinanceParty } from '../../api/analyticsApi';
import { financeParties } from '../../functions/financeEntries';
import { formatDate } from '../../functions/formatDate';
import { useFinanceReport } from '../../hooks/useFinanceReport';
import { Button } from '../Button/Button';
import { FinanceLedger } from '../FinanceLedger/FinanceLedger';
import { FinancePdfExport } from '../FinancePdfExport/FinancePdfExport';
import { FinanceTotals } from '../FinanceTotals/FinanceTotals';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import { SellerAmounts } from '../SellerAmounts/SellerAmounts';
import styles from './FinanceReport.module.css';

export function FinanceReport({ demoRunId, clock }: { demoRunId?: string; clock?: string }) {
    const view = useFinanceReport(demoRunId, clock);
    const point = view.point.currentData;
    const market = view.market.currentData;
    const seller = view.seller.currentData;
    const data = point ?? market ?? seller;
    const reports = [view.point, view.market, view.seller];
    const report = view.report;
    const title = report && `${financeParties[report.partyType]}${report.partyId ? ` · ${report.partyId}` : ''}`;
    const ledgerFilters = report && { ...report.period, demoRunId, partyType: report.partyType, partyId: report.partyId };
    const summary = <>
        {data && <p className={styles.hint}>Проводки учтены по {formatDate(data.postedThrough)}.</p>}
        {point && <>
            <FinanceTotals items={[{ label: 'Вознаграждение ПВЗ', value: point.rewardMinor }, { label: 'Операционные расходы', value: point.operatingCostMinor },
                { label: 'Удержания', value: point.deductionsMinor }, { label: 'Прибыль ПВЗ', value: point.profitMinor, emphasis: true }]}
                breakdown={point.breakdown} />
            <p className={styles.hint}>Выдано заказов: {point.issuedOrders}. Обработано возвратов: {point.processedReturns}.</p>
        </>}
        {market && <FinanceTotals items={[{ label: 'Доходы', value: market.revenueMinor }, { label: 'Расходы', value: market.expenseMinor },
            { label: 'Финансовый результат', value: market.profitMinor, emphasis: true }]} breakdown={market.breakdown} />}
        {seller && <SellerAmounts data={seller} />}
    </>;

    return (
        <div className={styles.workspace}>
            <form className={styles.filter} onSubmit={view.apply}>
                <FormField label="Участник">
                    <select value={view.partyType} onChange={(event) => view.selectParty(event.currentTarget.value as FinanceParty)}>
                        {Object.entries(financeParties).map(([id, title]) => <option key={id} value={id}>{title}</option>)}
                    </select>
                </FormField>
                {view.partyType === 'pvz' && <FormField label="Пункт выдачи">
                    <select name="partyId" required defaultValue="">
                        <option value="">Выберите ПВЗ</option>
                        {view.nodes.map(({ id, name }) => <option key={id} value={id}>{name}</option>)}
                    </select>
                </FormField>}
                {view.partyType === 'seller' && <FormField label="ID продавца">
                    <input name="partyId" placeholder="Например, seller-12" required maxLength={128} pattern="[A-Za-z0-9_\-]+" />
                </FormField>}
                {view.partyType === 'pvz' && view.points.hasNextPage && <Button variant="secondary" disabled={view.points.isFetching}
                    onClick={() => void view.points.fetchNextPage()}>Ещё ПВЗ</Button>}
                <FormField label="Начало периода"><input type="date" name="start" defaultValue={view.dates.start} required /></FormField>
                <FormField label="Конец периода"><input type="date" name="end" defaultValue={view.dates.end} required /></FormField>
                <Button type="submit">Показать отчёт</Button>
            </form>
            {view.validation && <p className={styles.error} role="alert">{view.validation}</p>}
            {view.points.isError && <QueryNotice error={view.points.error} onRetry={view.points.refetch} />}
            {reports.map((query, index) => query.isError ? <QueryNotice key={index} error={query.error} onRetry={query.refetch} /> : null)}
            {reports.some(({ isFetching }) => isFetching) && !data && <QueryNotice message="Загружаем отчёт…" />}
            {report && ledgerFilters && <>
                <div className={styles.reportHeader}>
                    <h2 className={styles.title}>{title}</h2>
                    <FinancePdfExport key={JSON.stringify(report)} title={`Финансовый отчёт · ${title}`} period={report.period} summary={summary}
                        enabled={Boolean(data) && !reports.some(({ isFetching, isError }) => isFetching || isError)}
                        journals={[{ title: 'Журнал проводок', filters: ledgerFilters }]} />
                </div>
                {summary}
                <FinanceLedger key={JSON.stringify(report)} title="Журнал проводок" filters={ledgerFilters} />
            </>}
        </div>
    );
}
