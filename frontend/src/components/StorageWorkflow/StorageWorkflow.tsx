import type { StorageCase } from '../../api/exceptionApi';
import { formatDate } from '../../functions/formatDate';
import { physicalSegments, storageStatuses } from '../../functions/exceptionRoutes';
import { formatMoney } from '../../functions/money';
import { useStorageWorkflow } from '../../hooks/useStorageWorkflow';
import { Button } from '../Button/Button';
import { DataTable } from '../DataTable/DataTable';
import { Dialog } from '../Dialog/Dialog';
import { FormField } from '../FormField/FormField';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import styles from './StorageWorkflow.module.css';

const titles = { decision: 'Решение по хранению', handed_over: 'Подтвердить вывоз', arrived: 'Подтвердить прибытие в хаб' };
const decisionLabels = { keep: 'Оставить на хранении', transfer_to_hub: 'Запланировать вывоз в хаб' };

export function StorageWorkflow({ cases, demoRunId, clock }: { cases: readonly StorageCase[]; demoRunId?: string; clock?: string }) {
    const view = useStorageWorkflow(cases, demoRunId, clock);
    const form = view.selection?.form;
    const rows = cases.map((item) => {
        const moving = item.status === 'transfer_planned' || item.status === 'in_transit';
        const movement = item.status === 'in_transit' ? 'arrived' : 'handed_over';

        return { id: item.id, cells: [<div><strong>{item.batchId}</strong><small>{item.pointId}</small></div>,
            `${item.ageDays} из ${item.thresholdDays} дн.`, formatMoney(item.cost.accruedMinor), formatMoney(item.cost.postedMinor),
            <div>{storageStatuses[item.status]}{item.nextCheckAt && <small>Проверка {formatDate(item.nextCheckAt)}</small>}</div>,
            <div>{decisionLabels[item.recommendation]}<small>{item.reason}</small></div>,
            item.status === 'open' || item.status === 'deferred' ? <Button variant="secondary" aria-haspopup="dialog" onClick={() => view.open(item, 'decision')}>Принять решение</Button>
                : moving && physicalSegments(item).length > 0 ? <Button variant="secondary" aria-haspopup="dialog" onClick={() => view.open(item, movement)}>{titles[movement]}</Button> : '—'] };
    });

    return (
        <div className={styles.workflow}>
            {view.message && <QueryNotice message={view.message} />}
            <DataTable title="Случаи длительного хранения" columns={['Партия', 'Возраст / порог', 'Начислено', 'Проведено', 'Статус', 'Рекомендация', 'Действие']}
                rows={rows} emptyMessage="Случаев длительного хранения нет." />
            <Dialog title={form ? titles[form] : 'Длительное хранение'} open={Boolean(form)} onClose={view.close} busy={view.command.pending}>
                {view.item && form && <form className={styles.form} onSubmit={(event) => void view.submit(event)}>
                    <p>Партия <strong>{view.item.batchId}</strong> · доступно {view.item.availableQuantity} шт.</p>
                    <fieldset disabled={view.command.pending}>
                        {form === 'decision' ? <>
                            <p>Сервер рекомендует {decisionLabels[view.item.recommendation].toLocaleLowerCase('ru')}. {view.item.reason}</p>
                            <FormField label="Дальнейшее действие">
                                <select value={view.decision} onChange={(event) => view.setDecision(event.currentTarget.value === 'keep' ? 'keep' : 'transfer_to_hub')}>
                                    <option value="keep">{decisionLabels.keep}</option>
                                    <option value="transfer_to_hub">{decisionLabels.transfer_to_hub}</option>
                                </select>
                            </FormField>
                            {view.decision === 'keep' && <FormField label="Повторная проверка, UTC"><input type="datetime-local" name="recheckAt" required /></FormField>}
                            <FormField label="Обоснование"><input name="reason" maxLength={500} required /></FormField>
                        </> : <FormField label="Сегмент вывоза">
                            <select name="segmentId" required defaultValue={view.segments.length === 1 ? view.segments[0].id : ''}>
                                <option value="">Выберите сегмент</option>
                                {view.segments.map(({ id, fromNodeId, toNodeId }) => <option key={id} value={id}>{fromNodeId} → {toNodeId}</option>)}
                            </select>
                        </FormField>}
                    </fieldset>
                    {(view.validation || view.command.error) && <p className={styles.error} role="alert">{view.validation || view.command.error}</p>}
                    <div className={styles.actions}>
                        <Button type="submit" disabled={view.command.pending}>{view.command.pending ? 'Ожидаем подтверждения…' : 'Подтвердить'}</Button>
                        <Button variant="secondary" disabled={view.command.pending} onClick={view.close}>Закрыть</Button>
                    </div>
                </form>}
            </Dialog>
        </div>
    );
}
