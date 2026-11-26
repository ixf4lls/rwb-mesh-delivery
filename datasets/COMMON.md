# Общие правила датасетов

## Типы ID и OpenAPI
- Все идентификаторы сущностей состоят только из цифр. Для полей API, где OpenAPI задаёт `type: string`, записывать ID как строки из цифр (например, `"123"`), а не JSON number: это сохраняет совместимость контракта. В CSV также хранить десятичные строки без знака, ведущих нулей и букв.
- Это относится к `city_id`, `pickup_point_id`, `seller_id`, `seller_sku`, `product_id`, `request_key/order_id`, `trip_id`, `segment_id`, `batch_id`, `return_id` и прочим полям-идентификаторам. Даты, текстовые ключи случаев и имена полями ID не являются.
- Разные датасеты используют непересекающиеся числовые пространства ID. Общие бизнес-справочники сценариев 1–5 наследуют одинаковые атрибуты, но ID выделяются в диапазоне конкретного датасета.
- Диапазоны: `demo=1`, `dense=2`, `peak=3`, `sparse=4`, `slow_returns=5`, `city150=6`, `russia10_smoke=7`, `yoshkar_ola_city=8`, `russia10_full=9`; диапазон набора начинается с `dataset_number * 1_000_000_000`. Смещения: `order/request=10_000_000`, `trip=50_000_000`, `product=100_000_000`, `seller_sku=200_000_000`, `seller=300_000_000`, `city=400_000_000`, `pickup_point=500_000_000`, `batch=600_000_000`, `return=700_000_000`, `segment=800_000_000`, `node=900_000_000`, `district=920_000_000`, `category=950_000_000`, `tariff=975_000_000`.

## Нормализация
- Строковые коды и отображаемые значения не дублировать в фактах. Использовать таблицы соответствий: `cities.csv(city_id, city_code, city_name, time_zone)`, `points.csv(pickup_point_id, city_id, point_code, point_name, ...)`, `sellers.csv(seller_id, seller_code, seller_name)`, `seller_skus.csv(seller_sku, seller_id, sku_value)`, `products.csv(product_id, seller_id, seller_sku, product_name, category, width_m, height_m, depth_m, ...)`.
- Факты содержат только numeric-string FK и атрибуты события. Ссылки в `items_json` используют `product_id` как строку из цифр.
- Геометрия товара: `width_m`, `height_m`, `depth_m` — конечные положительные числа в метрах. Не добавлять площадь основания или объём.

## Файлы, каталог и манифест
- Небольшие данные — UTF-8 CSV/JSON. Parquet применять только при наличии реального writer-а; не называть CSV файлом Parquet.
- В JSON-примерах для API сохранять `contract`, `schema`, `direction`; соответствовать типам и ограничениям OpenAPI. DatasetPage в `catalog.json` содержит только поля `Dataset`.
- Каждый готовый `datasets/<id>/` включает `manifest.json`, нормализованные справочники, факты и `expected.json`. Manifest указывает seed, период и часы, количества фактов, файлы, строки и SHA-256. Каталог помечает `ready` только реально созданные и проверенные наборы.
- `orders.csv`: один заказ на строку, `request_key` — числовая строка, `items_json` — неповторяющиеся numeric-string `product_id` и положительные количества. CSV/FK IDs проверяются как `^[0-9]+$`.

## Детерминизм и команды
- Seed по умолчанию 42. Производные значения зависят от seed и стабильного ключа; коллизии SKU разрешать конечным счётчиком попыток, не повторять неизменный кандидат бесконечным циклом.
- Общий CLI: `python3 scripts/datasets.py generate <id> --seed 42`, `python3 scripts/datasets.py validate [id]`, `python3 scripts/datasets.py estimate russia10_full`. Полный федеральный датасет запускать отдельно и вне репозитория.

- `points.csv` также содержит `rating` (1.0–5.0 с одним десятичным знаком), `load_level` (`низкая`, `средняя`, `высокая`) и положительную целую `capacity_cells`; значения детерминированы seed и ID точки. Для сценариев dense/peak storage-точки имеют 100 ячеек.
