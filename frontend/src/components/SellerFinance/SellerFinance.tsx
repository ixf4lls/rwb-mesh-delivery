import { formatDate } from '../../functions/formatDate';
import { useSellerEconomics } from '../../hooks/useSellerEconomics';
import { Button } from '../Button/Button';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import { SellerAmounts } from '../SellerAmounts/SellerAmounts';
import styles from './SellerFinance.module.css';

export function SellerFinance({ sellerId, demoRunId, clock }: { sellerId: string; demoRunId?: string; clock?: string }) {
    const { dates, economics, error, applyPeriod } = useSellerEconomics(sellerId, demoRunId, clock);
    const data = economics.currentData;

    return (
        <div className={styles.finance}>
            <form className={styles.filter} onSubmit={applyPeriod}>
                <FormField label="Начало периода"><input name="start" type="date" required defaultValue={dates.start} /></FormField>
                <FormField label="Конец периода"><input name="end" type="date" required defaultValue={dates.end} /></FormField>
                <Button variant="secondary" type="submit">Показать период</Button>
            </form>
            {error && <p className={styles.error} role="alert">{error}</p>}
            {economics.isError && <QueryNotice error={economics.error} onRetry={economics.refetch} />}
            {economics.isFetching && !data && <QueryNotice message="Загружаем финансовые показатели…" />}
            {data && <>
                <p className={styles.hint}>Проводки учтены по {formatDate(data.postedThrough)}.</p>
                <SellerAmounts data={data} />
            </>}
        </div>
    );
}
