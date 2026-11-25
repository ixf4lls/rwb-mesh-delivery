import { formatDate } from '../../functions/formatDate';
import { policyNames } from '../../functions/deliveryRoutes';
import { formatMoney } from '../../functions/money';
import { orderStatusLabels } from '../../functions/pvzOrders';
import { useDeliveryModel } from '../../hooks/useDeliveryModel';
import { useDemoScope } from '../../hooks/useDemoScope';
import { DeliveryControls } from '../DeliveryControls/DeliveryControls';
import { DeliveryDataset } from '../DeliveryDataset/DeliveryDataset';
import { DeliveryEvents } from '../DeliveryEvents/DeliveryEvents';
import { DeliveryFinancials } from '../DeliveryFinancials/DeliveryFinancials';
import { DeliveryGraph } from '../DeliveryGraph/DeliveryGraph';
import { NoticeInbox } from '../NoticeInbox/NoticeInbox';
import { QueryNotice } from '../QueryNotice/QueryNotice';
import { RouteAnalytics } from '../RouteAnalytics/RouteAnalytics';
import { RouteNodeDetails } from '../RouteNodeDetails/RouteNodeDetails';
import { RouteOptions } from '../RouteOptions/RouteOptions';
import styles from './DeliveryWorkspace.module.css';

function DeliveryModelPanel({ demoRunId, clock }: { demoRunId?: string; clock?: string }) {
    const model = useDeliveryModel(demoRunId);
    const order = model.order;
    const plan = model.route.currentData;
    const incidents = model.incidents.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const title = model.activeReturn ? 'Обратный маршрут' : order ? 'Маршрут заказа' : model.selectedOption ? policyNames[model.selectedOption.policy] : 'Логистическая сеть';
    const currentReasons = model.activeReturn ? model.activeReturnOption?.reasons : plan?.reasons;

    return (
        <>
            <div className={styles.layout}>
                <DeliveryControls model={model} />
                <div className={styles.center}>
                    <div className={styles.title}>
                        <h2>{title}</h2>
                        {model.activeReturn && <span>{model.activeReturn.id}</span>}
                        {!model.activeReturn && order && <span>{order.id} · {orderStatusLabels[order.status]}</span>}
                    </div>
                    {model.orderQuery.isError && <QueryNotice error={model.orderQuery.error} onRetry={model.orderQuery.refetch} />}
                    {model.route.isError && <QueryNotice error={model.route.error} onRetry={model.route.refetch} />}
                    {model.mode === 'order' && (model.orderQuery.isLoading || model.route.isLoading) && <QueryNotice message="Загружаем маршрут заказа…" />}
                    <DeliveryGraph key={model.graphKey} shipments={model.shipments} points={model.nodeMap} sellerIds={model.sellersByProduct}
                        incidents={model.activeReturn ? [] : incidents} order={model.activeReturn ? undefined : order} variant={model.variant}
                        onSelectPoint={model.selectNode} />
                    {model.shipments?.length ? <p className={styles.legend}>
                        {model.variant === 'hub' ? 'Штриховые линии · через хаб.' : model.variant === 'return' ? 'Зелёные линии · обратный маршрут.'
                            : 'Фиолетовые линии · маршрут. Зелёные · пройденные этапы. Красные · сбои.'} Нажмите на узел, чтобы открыть его данные.
                    </p> : null}
                    {plan && !model.activeReturn && <p className={styles.legend}>Версия {plan.revision} · прогноз готовности {formatDate(plan.expectedReadyAt)} · затраты {formatMoney(plan.totalEstimatedCostMinor)}.</p>}
                    {currentReasons?.length ? <ul className={styles.reasons}>{currentReasons.map(({ message }, index) => <li key={index}>{message}</li>)}</ul> : null}
                    {model.nodeId && <RouteNodeDetails key={model.nodeId} nodeId={model.nodeId} node={model.nodeMap.get(model.nodeId)} demoRunId={demoRunId} />}
                </div>
                <RouteOptions quote={model.availableQuote} selectedId={model.mode === 'order' ? plan?.selectedOptionId ?? '' : model.optionId}
                    canReserve={model.canReserve} busy={model.reservation.pending || model.quoteCommand.pending} error={model.reservation.error}
                    onSelect={model.chooseOption} onReserve={() => void model.reserveOrder()} />
            </div>
            <RouteAnalytics quote={model.availableQuote} shipments={model.shipments} points={model.nodeMap} />
            <DeliveryEvents model={model} demoRunId={demoRunId} clock={clock} />
            <NoticeInbox demoRunId={demoRunId} />
            {order && <DeliveryFinancials key={order.id} order={order} clock={clock} demoRunId={demoRunId} />}
            <details className={styles.criteria}>
                <summary>Условия модели</summary>
                <p>Команда предлагает учитывать свободные места, рейтинг ПВЗ от 4,5, нагрузку сотрудников, спрос и выгодность хранения. Предлагаемые пределы товара — 60 × 40 × 40 см и 20 кг.</p>
                <p>Эти пороги относятся к модели команды. Интерфейс показывает решение API и его причины. Прогноз прибыли по каждой альтернативе и изменение тарифов в котировке пока недоступны.</p>
            </details>
        </>
    );
}

export default function DeliveryWorkspace() {
    const { demo, demoRunId, scopeReady } = useDemoScope();

    return (
        <div className={styles.workspace}>
            {demo.isError && <QueryNotice error={demo.error} onRetry={demo.refetch} />}
            {demo.isLoading && <QueryNotice message="Загружаем активный демонабор…" />}
            {demo.currentData && <DeliveryDataset key={demo.currentData.id} run={demo.currentData} />}
            {scopeReady && <DeliveryModelPanel key={demoRunId ?? 'default'} demoRunId={demoRunId} clock={demo.currentData?.clock} />}
        </div>
    );
}
