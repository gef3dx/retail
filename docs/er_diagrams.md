# ER-диаграммы базы данных розничной торговли (РФ)

## 1. Основная схема (Core)

```mermaid
erDiagram
    organization ||--o{ warehouse : has
    organization ||--o{ employee : employs
    organization ||--o{ counterparty : works_with
    organization ||--o{ cash_register : owns
    organization ||--o{ purchase_order : creates
    organization ||--o{ receipt_document : receives
    organization ||--o{ sales_order : processes
    organization ||--o{ shipment_document : ships
    organization ||--o{ sales_receipt : issues
    organization ||--o{ purchase_book : fills
    organization ||--o{ sales_book : fills
    organization ||--o{ tax_accounting_register : maintains
    organization ||--o{ declaration_register : prepares

    warehouse ||--o{ warehouse_zone : contains
    warehouse ||--o{ warehouse_balance : stores
    warehouse ||--o{ movement_document : from_or_to
    warehouse ||--o{ inventory_document : inventories

    catalog_product ||--o{ product_packaging : has
    catalog_product ||--o{ product_price : has_price
    catalog_product ||--o{ product_batch : belongs_to_batch
    catalog_product ||--o{ marking_code_pool : has_codes
    catalog_product ||--o{ receipt_line : appears_in
    catalog_product ||--o{ shipment_line : appears_in
    catalog_product ||--o{ sales_order_line : appears_in
    catalog_product ||--o{ sales_receipt_item : sold_in
    catalog_product }o--|| catalog_category : categorized
    catalog_product }o--|| catalog_brand : branded
    catalog_product }o--|| product_status : has_status

    counterparty ||--o{ purchase_order : supplier
    counterparty ||--o{ receipt_document : supplier
    counterparty ||--o{ sales_order : buyer
    counterparty ||--o{ shipment_document : buyer

    employee ||--o{ warehouse : manages
    employee ||--o{ sales_receipt : cashier
    employee ||--o{ cash_shift : opens_closes

    receipt_document ||--o{ receipt_line : contains
    shipment_document ||--o{ shipment_line : contains
    sales_order ||--o{ sales_order_line : contains
    sales_receipt ||--o{ sales_receipt_item : contains
    sales_receipt ||--o{ receipt_marking_link : marks

    marking_code_pool ||--o{ receipt_marking_link : linked
    marking_code_pool ||--o{ marking_batch : grouped

    product_batch ||--o{ warehouse_balance : stock
