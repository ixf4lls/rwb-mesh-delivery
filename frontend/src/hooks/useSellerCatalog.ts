import { useState } from 'react';
import type { FormEvent } from 'react';
import { pvzApi } from '../api/pvzApi';
import { sellerApi } from '../api/sellerApi';
import type { ProductCreate } from '../api/sellerApi';
import { parseRubles } from '../functions/money';
import { useRepeatableRequest } from './useRepeatableRequest';

export function useSellerCatalog(sellerId: string, demoRunId?: string) {
    const catalog = pvzApi.useListProductsInfiniteQuery({ sellerId, demoRunId });
    const [open, setOpen] = useState(false);
    const [success, setSuccess] = useState('');
    const [validation, setValidation] = useState('');
    const [createProduct] = sellerApi.useCreateProductMutation();
    const creation = useRepeatableRequest((body: ProductCreate, key) => createProduct({ body, key, demoRunId }).unwrap());

    function openForm() {
        creation.clearError();
        setValidation('');
        setOpen(true);
    }

    async function submitProduct(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const form = event.currentTarget;
        const fields = new FormData(form);
        const value = (name: string) => String(fields.get(name) ?? '').trim();
        const sellerSku = value('sellerSku');
        const name = value('name');
        const category = value('category');
        const weightGrams = Number(value('weightGrams'));
        const slotsPerUnit = Number(value('slotsPerUnit'));
        const salePriceMinor = parseRubles(value('salePrice'));
        const unitCostMinor = parseRubles(value('unitCost'));

        if (!sellerSku || !name || !Number.isSafeInteger(weightGrams) || weightGrams < 1
            || !Number.isSafeInteger(slotsPerUnit) || slotsPerUnit < 1 || salePriceMinor === undefined || unitCostMinor === undefined) {
            setValidation('Укажите название, артикул, целые вес и число мест. В цене допускается до двух знаков после запятой.');
            return;
        }

        setValidation('');
        setSuccess('');
        const product = await creation.submit({ sellerId, sellerSku, name, category: category || undefined, weightGrams,
            slotsPerUnit, salePriceMinor, unitCostMinor });
        if (product) {
            form.reset();
            setOpen(false);
            setSuccess(`Товар «${product.name}» зарегистрирован.`);
        }
    }

    return { catalog, open, closeForm: () => setOpen(false), openForm, submitProduct, creation, validation, success };
}
