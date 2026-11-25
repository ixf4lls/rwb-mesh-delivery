import type { DemoRun } from '../../api/deliveryApi';
import { formatDate } from '../../functions/formatDate';
import { useDeliveryDataset } from '../../hooks/useDeliveryDataset';
import { Button } from '../Button/Button';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './DeliveryDataset.module.css';

export function DeliveryDataset({ run }: { run: DemoRun }) {
    const { items, datasets, action, apply } = useDeliveryDataset(run);

    return (
        <details className={styles.panel}>
            <summary>Демонаборы · {items.find(({ id }) => id === run.datasetId)?.name ?? run.datasetId}</summary>
            <p>Модельное время {formatDate(run.clock)}. Новый прогон загружает исходное состояние набора.</p>
            <form onSubmit={apply}>
                <FormField label="Набор данных">
                    <select name="datasetId" required defaultValue="" disabled={action.pending}>
                        <option value="">Выберите сценарий</option>
                        {items.map((item) => <option key={item.id} value={item.id} disabled={item.status !== 'ready'}>{item.name}</option>)}
                    </select>
                </FormField>
                <Button type="submit" disabled={action.pending || !items.some(({ status }) => status === 'ready')}>Начать новый прогон</Button>
                {datasets.hasNextPage && <Button variant="secondary" onClick={() => void datasets.fetchNextPage()} disabled={datasets.isFetching}>Ещё наборы</Button>}
            </form>
            {action.error && <p role="alert">{action.error}</p>}
            {datasets.isError && <QueryNotice error={datasets.error} onRetry={datasets.refetch} />}
        </details>
    );
}
