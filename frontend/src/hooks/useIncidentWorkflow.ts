import { useState } from 'react';
import type { FormEvent } from 'react';
import { exceptionApi } from '../api/exceptionApi';
import type { IncidentCreate } from '../api/exceptionApi';
import type { Order, RouteRevision } from '../api/pvzApi';
import { incidentTargets } from '../functions/exceptionRoutes';
import { useExceptionCommand } from './useExceptionCommand';
import { useRepeatableRequest } from './useRepeatableRequest';

export function useIncidentWorkflow(order: Order, route: RouteRevision | undefined, demoRunId?: string) {
    const [open, setOpen] = useState(false);
    const [shipmentId, selectShipment] = useState('');
    const [message, setMessage] = useState('');
    const [validation, setValidation] = useState('');
    const shipments = (order.status === 'issued' || order.status === 'canceled' ? undefined : route)?.shipments.filter((item) => order.shipments.some(({ id, status }) => id === item.id
        && status !== 'ready' && status !== 'issued' && status !== 'canceled')) ?? [];
    const shipment = shipments.find(({ id }) => id === shipmentId) ?? shipments[0];
    const targets = incidentTargets(shipment);
    const [register] = exceptionApi.useRegisterIncidentMutation();
    const creation = useRepeatableRequest((body: IncidentCreate, key) => register({ body, key, demoRunId }).unwrap());
    const routing = useExceptionCommand(demoRunId);

    function openForm() {
        setValidation('');
        setMessage('');
        creation.clearError();
        setOpen(true);
    }

    async function submit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const fields = new FormData(event.currentTarget);
        const target = targets.get(String(fields.get('target') ?? ''));
        const description = String(fields.get('description') ?? '').trim();

        if (!shipment || !target || !description) {
            setValidation('Выберите проблемный ресурс и опишите сбой.');
            return;
        }

        setValidation('');
        const result = await creation.submit({ orderId: order.id, shipmentId: shipment.id, type: target.type, failureTarget: target.failureTarget, description });
        if (result) {
            setOpen(false);
            setMessage(`Сбой ${result.id} зарегистрирован.`);
        }
    }

    async function reroute(id: string) {
        setMessage('');
        const result = await routing.submit({ kind: 'reroute', id, body: { expectedOrderVersion: order.version } });
        if (result) setMessage('Пересчёт подтверждён. Показываем последнюю версию маршрута и результат инцидента.');
    }

    return { open, openForm, close: () => setOpen(false), submit, message, validation, reroute, shipments, shipment, targets, selectShipment, creation, routing };
}
