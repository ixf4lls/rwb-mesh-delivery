import { useState } from 'react';
import type { FormEvent } from 'react';
import { pvzApi } from '../api/pvzApi';
import type { BatchReceipt } from '../api/pvzApi';
import { useCursorPage } from './useCursorPage';
import { useRepeatableRequest } from './useRepeatableRequest';

export function usePvzStock(pointId: string, demoRunId?: string) {
    const [receiving, setReceiving] = useState(false);
    const [success, setSuccess] = useState('');
    const [validation, setValidation] = useState('');
    const page = useCursorPage();
    const products = pvzApi.useListProductsInfiniteQuery({ demoRunId });
    const catalog = products.currentData?.pages.flatMap(({ items }) => items) ?? [];
    const productsById = new Map(catalog.map((product) => [product.id, product]));
    const batches = pvzApi.useListBatchesQuery({ pointId, demoRunId, cursor: page.cursor });
    const [receiveBatch] = pvzApi.useReceiveBatchMutation();
    const receipt = useRepeatableRequest((body: BatchReceipt, key) => receiveBatch({ body, key, demoRunId }).unwrap());

    function openReceipt() {
        receipt.clearError();
        setValidation('');
        setReceiving(true);
    }

    async function submitReceipt(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const form = event.currentTarget;
        const fields = new FormData(form);
        const productId = String(fields.get('productId') ?? '');
        const quantity = Number(fields.get('quantity'));
        const externalLotRef = String(fields.get('externalLotRef') ?? '').trim();

        if (!productsById.has(productId) || !Number.isSafeInteger(quantity) || quantity < 1) {
            setValidation('Выберите товар и укажите целое положительное количество.');
            return;
        }

        setSuccess('');
        setValidation('');
        const accepted = await receipt.submit({
            productId, storagePointId: pointId, quantity, externalLotRef: externalLotRef || undefined,
        });
        if (accepted) {
            form.reset();
            setReceiving(false);
            setSuccess(`Партия ${accepted.id} принята на хранение.`);
            page.reset();
        }
    }

    return { products, catalog, productsById, batches, page, receipt, receiving, success, validation, openReceipt, submitReceipt,
        closeReceipt: () => setReceiving(false) };
}
