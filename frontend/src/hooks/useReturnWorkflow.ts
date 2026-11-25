import { useState } from 'react';
import type { FormEvent } from 'react';
import type { ReturnCase, ReturnCommand } from '../api/exceptionApi';
import type { Order } from '../api/pvzApi';
import { physicalSegments } from '../functions/exceptionRoutes';
import { useExceptionCommand } from './useExceptionCommand';

type ReturnForm = 'create' | ReturnCommand['type'];

export function useReturnWorkflow(order: Order, selected: ReturnCase | undefined, demoRunId: string | undefined, onSelect: (id: string) => void) {
    const [form, setForm] = useState<ReturnForm | null>(null);
    const [target, setTarget] = useState<{ id: string; version: number }>();
    const [validation, setValidation] = useState('');
    const [message, setMessage] = useState('');
    const command = useExceptionCommand(demoRunId);
    const segments = selected ? physicalSegments(selected) : [];

    function open(type: ReturnForm) {
        setTarget(selected && { id: selected.id, version: selected.version });
        setValidation('');
        setMessage('');
        command.clearError();
        setForm(type);
    }

    async function submit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const values = new FormData(event.currentTarget);
        const reason = String(values.get('reason') ?? '').trim();
        setValidation('');

        if (form === 'create') {
            const returnPointId = String(values.get('returnPointId') ?? '');
            const requested = order.lines.map((line) => ({ orderItemId: line.id, quantity: Number(values.get(`quantity:${line.id}`)), max: line.quantity }));
            const lines = requested.filter(({ quantity }) => quantity > 0).map(({ orderItemId, quantity }) => ({ orderItemId, quantity }));
            if (!returnPointId || !reason || lines.length === 0 || requested.some(({ quantity, max }) => !Number.isSafeInteger(quantity) || quantity < 0 || quantity > max)) {
                setValidation('Укажите ПВЗ, причину и количество товара в пределах выданных строк.');
                return;
            }

            const result = await command.submit({ kind: 'createReturn', body: { orderId: order.id, returnPointId, reason, lines } });
            if (result) {
                setForm(null);
                if (result.resourceId) onSelect(result.resourceId);
                setMessage('Возврат оформлен. Обратный маршрут рассчитан сервером.');
            }
            return;
        }

        if (!form || !selected || !target || selected.id !== target.id) return;

        const expectedVersion = target.version;
        let body: ReturnCommand;
        if (form === 'inspected') {
            const lines = selected.lines.map(({ orderItemId, quantity }) => {
                const resellableQuantity = Number(values.get(`good:${orderItemId}`));
                return { orderItemId, resellableQuantity, quarantinedQuantity: quantity - resellableQuantity };
            });
            if (lines.some(({ resellableQuantity, quarantinedQuantity }) => !Number.isSafeInteger(resellableQuantity) || resellableQuantity < 0 || quarantinedQuantity < 0)) {
                setValidation('Количество годных единиц должно быть целым и не превышать объём возврата.');
                return;
            }
            body = { type: form, expectedVersion, lines };
        } else if (form === 'handed_over' || form === 'arrived') {
            const segmentId = String(values.get('segmentId') ?? '');
            if (!segments.some(({ id }) => id === segmentId)) {
                setValidation('Выберите физический сегмент обратного маршрута.');
                return;
            }
            body = { type: form, segmentId, expectedVersion };
        } else if (form === 'canceled') {
            if (!reason) {
                setValidation('Укажите причину отмены.');
                return;
            }
            body = { type: form, reason, expectedVersion };
        } else {
            body = { type: 'received_from_customer', expectedVersion };
        }

        const result = await command.submit({ kind: 'recordReturn', id: selected.id, body });
        if (result) {
            setForm(null);
            setMessage(form === 'inspected' ? 'Осмотр подтверждён. Остатки и начисления обновлены по данным сервера.' : 'Изменение возврата подтверждено.');
        }
    }

    return { open, form, close: () => setForm(null), submit, message, validation, command, segments };
}
