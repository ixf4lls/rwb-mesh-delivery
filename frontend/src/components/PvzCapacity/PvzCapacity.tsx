import { pvzApi } from '../../api/pvzApi';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './PvzCapacity.module.css';

export function PvzCapacity({ pointId, demoRunId }: { pointId: string; demoRunId?: string }) {
    const capacity = pvzApi.useGetPointCapacityQuery({ pointId, demoRunId });
    const data = capacity.currentData;

    if (capacity.isError) return <QueryNotice error={capacity.error} onRetry={capacity.refetch} />;
    if (!data) return <QueryNotice message="Загружаем вместимость ПВЗ…" />;

    const usedSlots = data.storageUsedSlots + data.ordinaryUsedSlots + data.pickupUsedSlots;

    return (
        <dl className={styles.metrics} aria-label="Вместимость ПВЗ">
            <div><dt>Общая вместимость</dt><dd>{data.totalSlots} мест</dd></div>
            <div><dt>Свободно для хранения</dt><dd>{data.availableStorageSlots} мест</dd></div>
            <div><dt>Свободно для выдачи</dt><dd>{data.availablePickupSlots} мест</dd></div>
            <div>
                <dt>Загрузка по местам</dt>
                <dd>{usedSlots} из {data.totalSlots}</dd>
                {data.totalSlots > 0 && <progress aria-label="Загрузка по местам" value={usedSlots} max={data.totalSlots} />}
            </div>
        </dl>
    );
}
