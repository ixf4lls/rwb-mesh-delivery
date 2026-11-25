import { skipToken } from '@reduxjs/toolkit/query';
import { useState } from 'react';
import type { FormEvent } from 'react';
import { useDispatch } from 'react-redux';
import { deliveryApi } from '../api/deliveryApi';
import type { QuoteRequest } from '../api/deliveryApi';
import { meshApi } from '../api/meshApi';
import { pvzApi } from '../api/pvzApi';
import { useCursorPage } from './useCursorPage';
import { useOperationConfirmation } from './useOperationConfirmation';
import { useRepeatableRequest } from './useRepeatableRequest';

export function useDeliveryModel(demoRunId?: string) {
    const scope = { demoRunId };
    const dispatch = useDispatch();
    const [cityId, selectCity] = useState('');
    const [pointId, selectPoint] = useState('');
    const [productId, selectProduct] = useState('');
    const [quantity, setQuantity] = useState('1');
    const [optionId, setOptionId] = useState('');
    const [orderId, selectOrder] = useState('');
    const [createdForQuote, setCreatedForQuote] = useState('');
    const [returnId, setReturnId] = useState('');
    const [nodeId, selectNode] = useState('');
    const [mode, setMode] = useState<'quote' | 'order'>('quote');
    const [inputError, setInputError] = useState('');
    const ordersPage = useCursorPage();
    const cities = deliveryApi.useListCitiesInfiniteQuery(scope);
    const points = pvzApi.useListPickupPointsInfiniteQuery(scope);
    const products = pvzApi.useListProductsInfiniteQuery(scope);
    const orders = pvzApi.useListOrdersQuery({ ...scope, cursor: ordersPage.cursor });
    const pointsList = points.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const productList = products.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const destinationPoints = pointsList.filter(({ node, acceptsOrders }) => node.active && acceptsOrders && (!cityId || node.cityId === cityId));
    const returnPoints = pointsList.filter(({ node, acceptsReturns }) => node.active && acceptsReturns).map(({ node }) => node);
    const nodeMap = new Map(pointsList.map(({ node }) => [node.id, node]));
    const effectivePointId = destinationPoints.some(({ node }) => node.id === pointId) ? pointId : '';
    const orderScope = mode === 'order' && orderId ? { orderId, demoRunId } : skipToken;
    const orderQuery = deliveryApi.useGetDeliveryOrderQuery(orderScope, { pollingInterval: 8000 });
    const order = orderQuery.currentData;
    const route = pvzApi.useGetOrderRouteQuery(order ? { ...scope, orderId: order.id, revision: order.routeRevision } : skipToken);
    const returns = deliveryApi.useListModelReturnsInfiniteQuery(orderScope, { pollingInterval: 15000 });
    const incidents = deliveryApi.useListModelIncidentsInfiniteQuery(orderScope, { pollingInterval: 15000 });
    const returnList = returns.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const activeReturn = returnList.find(({ id }) => id === returnId);
    const activeReturnOption = activeReturn?.route.options.find(({ id }) => id === activeReturn.selectedOptionId);

    const [requestQuote, quoteResult] = deliveryApi.useCreateQuoteMutation();
    const quoteCommand = useRepeatableRequest((body: QuoteRequest, key) => requestQuote({ ...scope, body, key }).unwrap());
    const quote = quoteResult.data;
    const selectedOption = quote?.options.find(({ id }) => id === optionId);
    const availableQuote = mode === 'quote' || order?.quoteId === quote?.id ? quote : undefined;
    const confirmOperation = useOperationConfirmation(demoRunId);
    const [reserve] = deliveryApi.useCreateDeliveryOrderMutation();
    const reservation = useRepeatableRequest(async (body: { quoteId: string; optionId: string }, key) => {
        const accepted = await reserve({ ...scope, body, key }).unwrap();
        const completed = await confirmOperation(accepted);
        if (!completed.resourceId) throw new Error('Сервер подтвердил команду без номера заказа. Повторите проверку той же команды.');

        dispatch(meshApi.util.invalidateTags(['Orders', 'Capacity', 'Batches']));
        return completed.resourceId;
    });
    const quotedItem = quote?.request.items[0];
    const quoteNeedsRefresh = Boolean(quote && mode === 'quote' && (quote.request.pickupPointId !== effectivePointId
        || quotedItem?.productId !== productId || quotedItem?.quantity !== Number(quantity)));
    const canReserve = Boolean(quote && selectedOption?.feasible && mode === 'quote' && !quoteNeedsRefresh && createdForQuote !== quote.id);

    async function calculate(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const itemCount = Number(quantity);
        if (!effectivePointId || !productList.some(({ id }) => id === productId) || !Number.isSafeInteger(itemCount) || itemCount < 1) {
            setInputError('Выберите ПВЗ, товар и целое положительное количество.');
            return;
        }

        setInputError('');
        selectNode('');
        setOrderIdForView('');
        setOptionId('');
        reservation.clearError();
        const result = await quoteCommand.submit({ pickupPointId: effectivePointId, items: [{ productId, quantity: itemCount }] });
        if (result) {
            const recommended = result.options.find(({ id, feasible }) => feasible && id === result.recommendedOptionId);
            setOptionId(recommended?.id ?? result.options.find(({ feasible }) => feasible)?.id ?? '');
        }
    }

    function setOrderIdForView(id: string) {
        selectOrder(id);
        setReturnId('');
        setMode(id ? 'order' : 'quote');
        selectNode('');
    }

    function chooseOption(id: string) {
        setOptionId(id);
        setMode('quote');
        setReturnId('');
        selectNode('');
    }

    async function reserveOrder() {
        if (!quote || !selectedOption?.feasible || !canReserve) return;

        const id = await reservation.submit({ quoteId: quote.id, optionId: selectedOption.id });
        if (id) {
            setCreatedForQuote(quote.id);
            setOrderIdForView(id);
        }
    }

    const shipments = mode === 'order' ? (activeReturn ? activeReturnOption?.shipments : route.currentData?.shipments) : selectedOption?.shipments;
    const variant = mode === 'order' ? (activeReturn ? 'return' : 'current') : selectedOption?.policy === 'hub_only' ? 'hub' : 'mesh';
    const graphKey = mode === 'order' ? `${orderId}:${route.currentData?.revision ?? ''}:${returnId}` : `${quote?.id}:${optionId}`;
    const sellersByProduct = new Map(productList.map(({ id, sellerId }) => [id, sellerId]));
    for (const { productId, sellerId } of order?.lines ?? []) sellersByProduct.set(productId, sellerId);

    return {
        cities, points, products, orders, ordersPage, destinationPoints, productList, cityId, selectCity, effectivePointId, selectPoint,
        productId, selectProduct, quantity, setQuantity, quoteNeedsRefresh,
        mode, quote, availableQuote, optionId, chooseOption, selectedOption, calculate, quoteCommand, inputError, reservation, reserveOrder, canReserve,
        order, orderId, orderQuery, route, setOrderIdForView, incidents, returns, returnList, returnId, activeReturn, activeReturnOption,
        selectReturn: (id: string) => { setReturnId(id); selectNode(''); },
        shipments, variant, graphKey, sellersByProduct, nodeMap, nodeId, selectNode, returnPoints,
    } as const;
}

export type DeliveryModel = ReturnType<typeof useDeliveryModel>;
