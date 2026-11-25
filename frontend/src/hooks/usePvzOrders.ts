import { useState } from 'react';
import { pvzApi } from '../api/pvzApi';
import type { Order } from '../api/pvzApi';
import type { OrderAction } from '../functions/pvzOrders';
import { useCursorPage } from './useCursorPage';
import { useOrderCommand } from './useOrderCommand';

export function usePvzOrders(pointId: string, demoRunId?: string) {
    const [status, setStatus] = useState<Order['status'] | ''>('');
    const [selectedId, selectOrder] = useState('');
    const [action, setAction] = useState<OrderAction | null>(null);
    const [success, setSuccess] = useState('');
    const page = useCursorPage();
    const orders = pvzApi.useListOrdersQuery({ pointId, demoRunId, status: status || undefined, cursor: page.cursor });
    const items = orders.currentData?.items ?? [];
    const order = items.find(({ id }) => id === selectedId) ?? items[0];
    const command = useOrderCommand(demoRunId);

    function selectStatus(value: Order['status'] | '') {
        page.reset();
        setStatus(value);
        selectOrder('');
    }

    function openAction(value: OrderAction) {
        command.clearError();
        setSuccess('');
        setAction(value);
    }

    async function confirmAction() {
        if (!action) return;

        const result = await command.submit(action);
        if (result) {
            setAction(null);
            setSuccess(`Операция по заказу ${action.orderId} подтверждена.`);
        }
    }

    return { status, selectStatus, orders, items, order, selectOrder, page, action, openAction, confirmAction, command, success,
        closeAction: () => setAction(null) };
}
