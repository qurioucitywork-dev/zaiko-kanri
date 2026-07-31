# M/R/C DTO マッピング（API契約確定前）

- 作成日: 2026-07-30
- 対象: 在庫管理システム
- ステータス: Draft / 非実装資料
- 参照:
  - `01-mock-logical-model.md`
  - `02-difference-matrix.md`
  - `03-d1-r2-physical-design.md`
  - `internal/web/*_handlers.go`
  - `internal/database/*.go`
  - `internal/database/migrations/000001`〜`000027`

## 1. 目的と非目的

本書は、次の三つの表現を混在させずに、後続のAPI設計で変換境界を確認するための
**意味マッピング**を定義する。

- `M`: モック `schema-diagram.html` / `APP_DATA` の外部項目
- `R`: 2026-07-29レビュー追加仕様の画面・操作項目
- `C`: 現行Goフォーム、Go構造体、SQLite列

`canonical DTO field` は、三者の意味を比較するための暫定的な標準名である。
この文書はHTTPパス、JSON wire format、必須制約、エラーコード、DB変更を確定しない。
API・コード・migration・UIの変更も行わない。

## 2. 読み方と共通規則

### 2.1 列

| 列 | 意味 |
|---|---|
| 画面/API/DB | 項目が現れる画面、想定するAPI境界、現行DB保存先 |
| external field | MまたはRで利用者・画面から見える項目名 |
| canonical DTO field | 境界間で同じ意味として扱う暫定標準名 |
| current field | Cのフォーム名 / Goフィールド / DB列。複数ある場合は `→` で変換順を示す |
| 型 | 標準化候補。金額は原則 `int64 minor`、日付は `date string`、日時は `RFC3339 timestamp` |
| nullable | `No`、`Yes`、`Conditional`。現時点の意味でありAPI契約ではない |
| source | `M`、`R`、`C`、または組合せ |
| decision / P0依存 | 仕様決定が必要な事項。`—` は現行意味を維持可能 |
| 互換性影響 | 既存フォーム、Go型、DB、既存データへの影響 |

### 2.2 標準型の候補

| 意味 | canonical表現候補 | 注意 |
|---|---|---|
| 内部ID | `string` | アプリケーション生成IDを維持し、画面に直接公開しない |
| 業務番号 | `string` | `product_code`、`slip_number`等。内部IDと分離 |
| 日付 | `string(date)` | `YYYY-MM-DD` |
| 日時 | `string(timestamp)` | UTC RFC3339。表示時のみAsia/Tokyo |
| 金額 | `int64(minor)` | 通貨と必ず組にする。JavaScript境界は安全整数範囲を要確認 |
| 為替 | `int64(scaled)` + `int64(scale)` | 浮動小数点へ無条件変換しない |
| 真偽値 | `bool` | SQLite/D1では0/1へ変換 |
| 状態 | `string(enum)` | DB値と日本語表示ラベルを分離 |
| 複数値 | `array<T>` | 現行CSV文字列からの変換は後方互換が必要 |

### 2.3 横断項目

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 全認証済み画面 / 全内部API / 主要テーブル | 画面には表示しない | `organization_id` | 認証context → 各Go入力 `OrganizationID` → 各テーブル `organization_id` | string | No | C | API入力から任意指定させない | 現行スコープを維持。Mには存在しないが削除不可 |
| 更新系画面 / command / 監査列 | 操作者 | `actor_user_id` | session user → `CreatedBy` / `ActorID` / `ActorUserID` → `created_by`等 | string | No | C | — | Mの `createdBy` 表示名とIDを混同しない |
| 一覧・詳細 / read DTO / 各テーブル | ステータス | `status_code` | Go `Status` → DB `status` / `inventory_status` | string(enum) | No | M/R/C | 状態対応表と許可遷移の確定 | 既存DB値を日本語ラベルへ置換しない |
| 一覧・詳細 / read DTO | 日本語ステータス表示 | `status_label` | template側の表示変換 | string | No | M/R/C | `status_code`ごとの表示辞書 | 表示専用。DBへ保存しない |
| 更新API | 変更理由・備考 | `change_reason` | `memo` / `change_memo` / `reason` / `notes` → revision/audit列 | string | Conditional | R/C | 操作別必須条件 | 既存の複数フォーム名を一括改名すると互換性あり |
| 一覧API | 全件表示 | `page_size`または`show_all` | query `show_all=1`、`ProductFilter.PageSize` | int / bool | Yes | R/C | 最大件数・ページング方式 | 大量データ時の上限が必要。DB変更なし |

## 3. 商品・在庫

### 3.1 商品／在庫 read・write DTO

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 在庫一覧・詳細・商品登録 / product DTO / `products` | 管理番号、商品コード / `inventory.code` | `product_code` | form `product_code` → Go `ProductCode` / `RequestedProductCode` → `products.product_code` | string | No | M/R/C | 採番は現行維持 | Mの`code`をそのままDB列名にしない |
| 商品登録・詳細 / product DTO / `products` | SKU / `inventory.sku`相当 | `sku` | form `sku` → Go `SKU` → `products.sku` | string | Yes | R/C | Mの正式項目として扱うか | 空文字運用をnullableへ変える場合は互換変換 |
| 在庫一覧・詳細 / product DTO / `products` | ブランド | `brand_name` | form `brand` → Go `Brand` → `products.brand` | string | No | M/R/C | ブランドID正規化はP1 | 当面は表示スナップショット文字列を維持 |
| 在庫一覧・詳細 / product DTO / `products` | モデル名 | `model_name` | M `model`; Go `ProductType`; DB `products.product_type` | string | No | M/C | `model`と`product_type`の語義確認 | 現行ゲスト読取では`model_name`と`reference_number`の割当も要監査 |
| 在庫一覧・詳細 / product DTO / `products` | 型番（Ref.） | `reference_number` | form `model_number` → Go `ModelNumber` → `products.model_number` | string | Yes | M/R/C | — | Mの`ref`から名称変換 |
| 在庫一覧・詳細 / product DTO / `products` | シリアル | `serial_number` | form `serial_number` → Go `SerialNumber` → `products.serial_number` | string | Yes | M/R/C | 重複許可理由は現行維持 | 空文字をNULLへ変える場合は移行必要 |
| 在庫一覧・詳細 / product DTO / `products` | 仕入先 | `supplier_id`, `supplier_name` | form `supplier_id` → Go `SupplierID/Name` → `products.supplier_id` + `suppliers.name` | string pair | Conditional | M/R/C | Mのsupplier codeとCの内部IDの境界 | writeはID、readはID＋表示名を推奨 |
| 在庫一覧・詳細 / product DTO / `purchase_slips`経由 | 仕入担当者 | `buyer_id`, `buyer_name` | form/query `buyer_id` → handler/Go `BuyerID/Name` → `purchase_slips.created_by`（`purchase_slip_lines`経由で商品にJOIN） | string pair | Yes | M/R/C | buyerの意味が仕入担当者か販売先かを分離 | 現行は商品列ではなく仕入伝票の起票者を担当者として解決する。UI都合で`products.buyer_id`を追加しない |
| 在庫一覧・詳細 / product DTO / `products` | 仕入日 | `purchase_date` | form/query `purchase_date` → Go `PurchaseDate` → `products.purchase_date` | date string | No | M/R/C | — | 形式を`YYYY-MM-DD`へ統一 |
| 在庫一覧・詳細 / product DTO / `products` | 仕入金額 | `purchase_amount_minor` | form `cost_amount` → parse → Go `CostAmountMinor` → `products.cost_amount_minor` | int64(minor) | No | M/R/C | — | M `purchasePrice`はJPY整数前提に見えるため通貨補完必須 |
| 同上 | 仕入通貨 | `purchase_currency` | form `cost_currency` → Go `CostCurrency` → `products.cost_currency` | string(enum) | No | R/C | JPY/USDの画面運用 | Mからは既定JPYとしてのみ補完可 |
| 在庫一覧・詳細 / product DTO / `products` | 売価 | `base_sale_amount_minor` | form `base_sale_price` → Go `BaseSalePriceMinor` → `products.base_sale_price_minor` | int64(minor) | No | M/R/C | 為替・価格の適用時点 | M `salePrice`との互換変換 |
| 同上 | 売価通貨 | `base_sale_currency` | form `base_sale_currency` → Go `BaseSaleCurrency` → `products.base_sale_currency` | string(enum) | No | R/C | P0 為替仕様 | Mからは既定JPYか不明。推測で固定しない |
| 在庫一覧・詳細 / product DTO / `products` | ステータス | `inventory_status` | form/query `inventory_status/status` → Go `InventoryStatus` → `products.inventory_status` | string(enum) | No | M/R/C | 日本語表示との対応表 | Cの7状態を維持 |
| 商品詳細 / product DTO / `products` | 素材（本体） | `material_text` | form `material` → Go `Material` → `products.material_text` | string | Yes | M/R/C | マスタID化はP1 | 既存文字列を失わない |
| 商品詳細 / product DTO / `products` | 駆動方式 | `movement_text` | form `movement` → Go `Movement` → `products.movement_text` | string | Yes | M/R/C | マスタID化はP1 | 同上 |
| 商品詳細 / product DTO / `products` | コンディション | `condition_text` | form `condition` → Go `Condition` → `products.condition_text` | string | Yes | M/R/C | 状態コード化はP1 | M code/nameと現行文字列の対応が必要 |
| 商品詳細 / product DTO / `products` | ベルト素材 | `belt_material_text` | form `belt_material` → Go `BeltMaterial` → `products.belt_material_text` | string | Yes | M/R/C | — | 表示スナップショット維持 |
| 商品詳細 / product DTO / `products` | 文字盤 | `dial_text` | form `dial` → Go `Dial` → `products.dial_text` | string | Yes | R/C | — | M `note`との混同禁止 |
| 商品詳細 / product DTO / `products` | 特徴・備考 | `features_text` | form `features` → Go `Features` → `products.features_text` | string | Yes | M/R/C | `note`と`comment`の公開範囲 | 社内コメントを外部DTOへ出さない |
| 商品詳細 / internal DTO / `products` | コメント（社内専用） | `internal_comment` | form `internal_comment` → Go `InternalComment` → `products.internal_comment_text` | string | Yes | M/R/C | 公開DTOからの除外 | ゲスト公開へ漏らさない |
| 商品登録・一覧 / product DTO / `products` | BOX | `box_text`（暫定） | form/query `box` → Go `Box` → `products.box_text` | string | Yes | R/C | P0 BOX正本: textか`guest_box_products`か | 現行検索互換のため即時削除不可 |
| 商品詳細 / product DTO / `products` | 付属品 | `accessory_codes[]`（候補） | form `accessories[]` → Join → Go `Accessories` → `products.accessories` CSV文字列 | array<string> | Yes | M/R/C | P1 正規化・区切り・未知値 | API境界でarray、DB adapterで現行文字列へ変換する候補 |
| 商品詳細 / media DTO / `product_images` | 商品画像 | `images[]` | Go `Images[]/ProductImage` → `product_images.*` | array<object> | Yes | M/C | R2/S3 object key仕様 | M URL配列へ戻さず、read時だけ配信URLを生成 |

### 3.2 在庫検索 DTO

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 在庫一覧 / search request / `products` | 管理番号 | `query`または`product_code` | query `q` → `ProductFilter.Query` | string | Yes | M/R/C | 複合検索か厳密検索か | 現行`q`互換を維持 |
| 同上 | ブランド | `brand_name` | query `brand` → `ProductFilter.Brand` | string | Yes | M/R/C | 将来brand ID化 | 文字列検索互換 |
| 同上 | 型番 | `reference_number` | query `model_number` → `ProductFilter.ModelNumber` | string | Yes | M/R/C | — | external名との差のみ |
| 同上 | シリアル | `serial_number` | query `serial_number` → `ProductFilter.SerialNumber` | string | Yes | M/R/C | — | — |
| 同上 | SKU | `sku` | query `sku` → `ProductFilter.SKU` | string | Yes | R/C | — | — |
| 同上 | 仕入先 | `supplier_id` | query `supplier_id` → `ProductFilter.SupplierID` | string | Yes | M/R/C | code入力時のID解決 | APIがcodeかIDか未確定 |
| 同上 | 仕入担当者 | `buyer_id` | query `buyer_id` → `ProductFilter.BuyerID` | string | Yes | R/C | 用語整理 | — |
| 同上 | BOX | `box_code` | query `box` → `ProductFilter.Box` → `products.box_text` | string | Yes | R/C | P0 BOX正本 | BOX1〜10の表示codeと内部IDを分離 |
| 同上 | 付属品 | `accessory_code` | query `accessory` → `ProductFilter.Accessory` | string | Yes | R/C | CSV文字列検索の限界 | 正規化前は完全一致保証不可 |
| 同上 | 仕入日From/To | `purchase_date_from/to` | query同名 → `ProductFilter.*` | date string | Yes | M/R/C | — | — |
| 同上 | ステータス | `inventory_status` | query `status` → `ProductFilter.Status` | string(enum) | Yes | M/R/C | UI label対応 | — |

## 4. 仕入

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 仕入登録・伝票詳細 / purchase DTO / `purchase_slips` | 仕入伝票番号 / `purchaseSlips.id` | `purchase_slip_number` | Go `SlipNumber` → `purchase_slips.slip_number` | string | No | M/R/C | Mのidを業務番号として解釈 | 内部`id`と分離 |
| 同上 | 仕入日 | `purchase_date` | form `purchase_date` → Go `PurchaseDate` → `purchase_slips.purchase_date` | date string | No | M/R/C | — | — |
| 同上 | 仕入先 | `supplier_id`, `supplier_name` | form `supplier_id` → Go `SupplierID/Name` → `supplier_id` JOIN | string pair | No | M/R/C | code/内部IDのwire選択 | writeはID、readは表示名を補完 |
| 同上 | 仕入担当者 | `buyer_id`, `buyer_name` | 現行は主に`created_by` / `CreatedByName` | string pair | Conditional | M/R/C | 業務担当者と起票者の同一性 | 同一と推測して固定しない |
| 同上 | 備考 | `notes` | form `notes` → Go `Notes` → `purchase_slips.notes` | string | Yes | R/C | — | — |
| 同上 | ステータス | `status_code` | Go `Status` → `purchase_slips.status` | string(enum) | No | M/R/C | M状態対応 | C `draft/confirmed/cancelled`維持 |
| 仕入明細 / purchase line DTO / `purchase_slip_lines` | No. | `line_number` | index+1 → `line_number` | int | No | R/C | 削除後採番は表示/保存双方確認 | 行IDと混同しない |
| 同上 | 行数・数量 | `quantity` | form `quantity[]` → Go `Quantity` → `quantity` | int | No | R/C | 商品単位か同一仕様複数個か | 現行は確定時に個体商品を生成 |
| 同上 | 商品コード | `requested_product_code` | form `product_code[]` → Go `ProductCode` → `requested_product_code` | string | Yes | M/R/C | 採番との優先順位 | 空欄時自動採番を維持 |
| 同上 | SKU | `sku` | form `sku[]` → Go `SKU` → `sku` | string | Yes | M/R/C | — | — |
| 同上 | 仕入金額 | `unit_cost_minor` | form amount parse → Go `UnitCostMinor` → `unit_cost_minor` | int64(minor) | No | M/R/C | — | 表示文字列から整数へ変換 |
| 同上 | 売価 | `base_sale_price_minor` | Go `BaseSalePriceMinor` → `base_sale_price_minor` | int64(minor) | No | M/R/C | 通貨・税との関係 | 後付列を維持 |
| 同上 | 商品詳細 | `product_detail` | Go `Brand`等 → 複数line列 | object | Conditional | M/R/C | objectの正式schema | MのネストobjectをDB JSONへ一括保存しない |
| 仕入修正 / command / revision | 修正メモ | `change_reason` | form `memo` → `UpdatePurchaseSlipInput.Memo` → `purchase_slip_revisions.memo` | string | Conditional | R/C | 修正時必須条件 | 既存履歴を維持 |
| 仕入CSV / import-export DTO | CSV取込・出力 | `purchase_csv_row` | 現行UI/handlerの各明細フォームへ変換 | object | — | R/C | P0 CSV列・encoding・原子性 | CSV仕様確定までAPI契約化しない |

## 5. 売上・通貨・税

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 売上登録・伝票詳細 / sales DTO / `sales_slips` | 売上伝票番号 / `sales.id` | `sales_slip_number` | form `slip_number` → Go `SlipNumber` → `sales_slips.slip_number` | string | No | M/R/C | 採番現行維持 | 内部IDと分離 |
| 同上 | 売上日 | `sales_date` | form `sales_date` → Go `SalesDate` → `sales_slips.sales_date` | date string | No | M/R/C | — | — |
| 同上 | 販売先 | `customer_id`, `customer_name` | form `customer_name` → Go `CustomerName` → `sales_slips.customer_name`; master/guest IDなし | string pair | Conditional | M/R/C | P0 販売先正本 | 現行自由文字列を失わない |
| 同上 | 住所・連絡先・適格番号 | `billing_profile` | Go `CustomerAddress/Phone/QualifiedInvoiceNumber` → `sales_slips.*` | object | Yes | R/C | master snapshotか伝票snapshotか | 請求書再現性のため伝票snapshot候補 |
| 同上 | 備考 | `notes` | form `notes` → Go `Notes` → `sales_slips.notes` | string | Yes | M/R/C | — | — |
| 売上明細 / sales line DTO / `sales_lines` | 管理番号 | `product_id`, `product_code` | form `product_id[]` → Go `ProductID`; JOIN `products.product_code` | string pair | No | M/R/C | 検索入力はcode、writeは内部IDか | resolver境界が必要 |
| 同上 | 数量 | `quantity` | form `quantity[]` → Go `Quantity` → `sales_lines.quantity` | int | No | M/R/C | 個体商品で数量>1の扱い | 現行制約を維持 |
| 同上 | 販売金額 | `unit_price_minor` | form `unit_price[]` → Go `UnitPriceMinor` → `sales_lines.unit_price_minor` | int64(minor) | No | M/R/C | — | — |
| 同上 | 円・ドル | `sale_currency` | form `currency[]`候補; 現行handlerはJPY固定 → Go `Currency` → `sale_currency` | string(enum) | No | R/C | P0 通貨選択・固定時点 | DBは対応済み、UI/handler互換追加が必要 |
| 同上 | 為替snapshot | `exchange_rate_snapshot` | Go `ExchangeRate*` → `sales_lines.exchange_rate_*` | object | Conditional | C | P0 取得元・固定時点・丸め | 過去伝票の数値snapshotを保持 |
| 同上 | JPY換算額 | `converted_total_jpy_minor` | Go `ConvertedTotalJPY` → `converted_total_jpy` | int64(minor) | Conditional | C | P0換算規則 | Mのtotalと直接同一視しない |
| 売上ヘッダー / sales DTO / 未保存 | 通常・免税 | `tax_mode` | form `tax_mode`をhandlerで検証するがDB保存なし | string(enum) | No | R/C | **P0 保存単位・税計算・表示** | 現状は送信後消失。API確定禁止 |
| 同上 | 小計・税・合計 | `subtotal_minor`, `tax_minor`, `total_minor` | 現行 `TotalJPY`は明細合計; 税列なし | int64(minor) | Conditional | R/C | **P0 税率・端数・免税** | 導出値かsnapshotか未決定 |
| 売上修正 / command / revision | 修正メモ | `change_reason` | form `memo` → `UpdateSalesSlipInput.Memo` → revision | string | Conditional | R/C | — | — |

## 6. 出荷

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 出荷登録・詳細 / shipment DTO / `shipment_slips` | 出荷伝票番号 / `shipments.id` | `shipment_number` | Go `ShipmentNumber` → `shipment_slips.shipment_number` | string | No | M/R/C | 採番現行維持 | 内部IDと分離 |
| 同上 | 出荷日 | `shipment_date` | form `shipment_date` → Go `ShipmentDate` → DB同名 | date string | No | M/R/C | — | — |
| 同上 | 出荷先 | `recipient_id`, `recipient_name` | form `recipient_name` → Go `RecipientName` → DB同名 | string pair | Conditional | M/R/C | 販売先正本と紐付け | 現行自由文字列を失わない |
| 同上 | 住所・連絡先 | `recipient_address`, `recipient_phone` | Go fields → `shipment_slips.recipient_*` | string | Yes | R/C | master snapshotか伝票snapshotか | 既存列維持 |
| 同上 | 配送番号 | `tracking_number` | Go `TrackingNumber` → `shipment_slips.tracking_number` | string | Yes | R/C | 処理済み前の入力禁止条件 | 仕入返品のdelivery numberと名称統一要否 |
| 出荷明細 / shipment line DTO / `shipment_lines` | 商品 | `product_id`, `product_code` | form `product_id[]` → Go `ProductID`; JOIN product code | string pair | No | M/R/C | — | — |
| 同上 | 点数 | `quantity` | Go `Quantity` → `shipment_lines.quantity` | int | No | M/R/C | — | — |
| 同上 | 卸値・金額 | `wholesale_amount_minor` | Go `WholesalePriceMinor` → `shipment_lines.wholesale_price_minor` | int64(minor) | No | M/R/C | 売上金額との意味差 | M `wholesale`から名称変換 |
| 売上連携 / allocation DTO / `sales_shipment_allocations` | 元売上伝票 | `sales_line_id`, `allocated_quantity` | DB `sales_line_id`, `shipment_line_id`, `allocated_quantity` | object | Conditional | R/C | P0 売上→出荷→返品の最終連携 | Mは直接参照を持たない |
| 出荷修正 / command / revision | 修正メモ | `change_reason` | form `memo` → `UpdateShipmentSlipInput.Memo` → revision | string | Conditional | R/C | — | — |

## 7. 返品・持ち帰り・請求書

### 7.1 売上返品／持ち帰り

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 返品一覧・詳細 / sales return DTO / `return_takehome_items` | 返品伝票番号 / `salesReturns.id` | `sales_return_number` | 現行はheaderなし。集約キーは`SalesSlipID`、各item `ID` | string | Conditional | M/R/C | **P0 header新設かview集約か** | APIで架空の永続IDを確定しない |
| 同上 | 元売上伝票 | `sales_slip_id`, `sales_slip_number` | `sales_slip_id` JOIN `sales_slips.slip_number` | string pair | No | M/R/C | — | — |
| 同上 | 返品日 | `return_date` | form `return_date` → `return_takehome_items.return_date` | date string | Conditional | M/R/C | header/明細どちらの属性か | 複数明細で日付差があり得る |
| 同上 | 販売先 | `customer_name` | JOIN `sales_slips.customer_name` | string | No | M/R/C | 販売先正本 | 現行snapshot表示 |
| 同上 | 返品理由 | `reason` | form `reason` → item `reason` | string | Yes | M/R/C | header/明細単位 | — |
| 同上 | 返品/持ち帰り区分 | `action_type` | form `action_type` → `return_takehome_items.action_type` | string(enum) | No | R/C | `return/take_home`表示対応 | M `sales.items[].returnType`との変換 |
| 同上 | 処理状態 | `status_code` | `return_takehome_items.status` | string(enum) | No | M/R/C | 集約status規則 | item単位からheader statusを導出中 |
| 同上 | 返品商品 | `items[]` | Go `ReturnTakehomeItem[]` → item rows + JOIN sales/product | array<object> | No | M/R/C | P0 集約単位 | 現行正規化を維持 |
| 在庫戻し / command / item+product | 確認後のコンディション | `restore_condition` | form `condition_{id}` → `ReturnRestoreItemInput.Condition` → `products.condition_text`更新。返品明細側は操作メモ・監査項目を保持し、専用`restore_condition_text`列は現行にない | string | No | R/C | 在庫更新の最終遷移 | APIで専用保存列の存在を前提にしない。commandは一業務transactionに閉じる |
| 同上 | 数量確認 | `restore_quantity` | form `quantity_{id}` → Go `Quantity` | int | No | R/C | 個体商品数量規則 | — |
| 同上 | BOX | `restore_box_code` | form `box_{id}` → Go `Box` → `restore_box_text` | string | Yes | R/C | P0 BOX正本 | text互換が必要 |
| 同上 | 在庫戻し日時 | `inventory_restored_at` | `return_takehome_items.inventory_restored_at` | timestamp | Yes | C | — | 完了状態と分離 |

### 7.2 仕入返品

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 仕入返品一覧・詳細 / purchase return DTO / `purchase_return_slips` | 返品伝票番号 / `purchaseReturns.id` | `purchase_return_number` | Go `ReturnNumber` → `return_number` | string | No | M/R/C | — | 内部IDと分離 |
| 同上 | 元仕入伝票 | `purchase_slip_id`, `purchase_slip_number` | `purchase_slip_id` JOIN purchase slip | string pair | Yes | R/C | 元伝票なしを許すか | 現行DB nullable |
| 同上 | 返品日 | `return_date` | form `return_date` → Go `ReturnDate` → DB同名 | date string | No | M/R/C | — | — |
| 同上 | 仕入先 | `supplier_id`, `supplier_name` | 現行headerは`supplier_name` snapshotのみ | string pair | Conditional | M/R/C | P1 supplier ID保持 | 既存伝票再現のためname snapshot維持 |
| 同上 | 返品理由・備考 | `reason`, `notes` | form同名 → DB同名 | string | Yes | M/R/C | — | — |
| 同上 | 金額合計 | `total_amount_minor`, `currency` | Go `AmountJPY` → `amount_jpy` | int64 + enum | No | M/R/C | JPY以外を許すか | 現行はJPY固定 |
| 同上 | 配送番号 | `tracking_number` | form `delivery_number` → Go `DeliveryNumber` → DB同名 | string | Yes | M/R/C | 用語と入力可能状態 | 現行form互換を維持 |
| 返品明細 / line DTO / `purchase_return_lines` | 商品コード・SKU・ブランド・モデル・金額 | `items[]` | Go `PurchaseReturnLine[]` → DB line列 | array<object> | No | M/R/C | item status定義 | Mのref/serial/trackingNo不足を別途補完検討 |
| 同上 | 明細状態 | `item_status` | JOIN product `inventory_status`をGo `Status`へ格納 | string(enum) | Conditional | C | 返品処理状態と在庫状態を分離 | 現在名称が曖昧 |

### 7.3 請求書

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 請求書preview / invoice read DTO | 発行元会社情報 | `issuer` | `invoiceCompany`の既定値; `master_records(category=company)`未接続 | object | No | M/R/C | **P0/P1 自社情報正本とsnapshot時点** | 現行ハードコードをAPI契約に固定しない |
| 同上 | 宛先 | `recipient` | 売上/仕入返品のcustomer/supplier snapshot | object | No | M/R/C | 販売先正本整理 | 過去帳票再現性に注意 |
| 同上 | 明細 | `lines[]` | `salesInvoiceLine` / `purchaseReturnInvoiceLine` | array<object> | No | M/R/C | 通貨・税表示 | — |
| 同上 | 通常・免税 | `tax_mode` | 現行永続なし | string(enum) | Conditional | R | **P0 税仕様** | 推測で通常扱いしない |
| 同上 | 通貨 | `currency` | sales lineは保持、purchase returnはJPY固定 | string(enum) | No | R/C | P0 複数通貨帳票 | — |
| 同上 | 小計・税・合計 | `subtotal_minor`, `tax_minor`, `total_minor` | 現行は主にtotal | int64(minor) | Conditional | R/C | **P0 税・丸め** | 導出/保存の責務未確定 |
| 発行・印刷 command / invoice state | 発行・印刷 | `issued_at/by`, `printed_at/by` | 各返品テーブルのinvoice列 | timestamp + user ID | Yes | M/R/C | 発行と印刷を同一操作にするか | M boolへ戻さず履歴を維持 |

## 8. マスタ・BOX・ゲスト公開

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| マスタ一覧・編集 / master DTO / `suppliers`または`master_records` | コード | `record_code` | form `code` → Go `Code` → `supplier_code`または`record_code` | string | No | M/R/C | category別正本 | adapterでtable差を吸収 |
| 同上 | 名称 | `name` | form/Go/DB `name` | string | No | M/R/C | — | — |
| 同上 | 住所・連絡先・適格番号 | `address`, `contact`, `invoice_registration_number` | form同名 → Go fields → DB同名 | string | Yes | M/R/C | — | 空文字互換 |
| 同上 | 分類固有項目 | `details` | form allowlist → `Details map` → `details_json` | object | Yes | R/C | category別schema/version | 任意JSONを無検証で外部公開しない |
| BOX一覧 / box DTO / `guest_boxes` | BOX番号 / `boxes.no` | `box_number`, `box_code` | Go `Number/Code` → `box_number`; codeは`BOX%d`導出 | int + string | No | M/R/C | — | codeを永続IDにしない |
| 同上 | BOX名 | `box_name` | form `box_name` → Go `Name` → DB同名 | string | Yes | M/R/C | — | — |
| BOX商品紐付け / relation DTO / `guest_box_products` | BOX商品 | `product_ids[]` | form `product_id[]` → relation rows | array<string> | Yes | R/C | `products.box_text`との責務 | P0 BOX正本 |
| BOX公開 / publication DTO / `guest_box_drafts/publications` | 公開先 | `company_ids[]` | company/box matrix → draft/publication rows | array<string> | Yes | M/R/C | **P0 公開・差替え・履歴** | M `publicTo`から正規化変換 |
| ゲスト商品一覧 / published DTO / `guest_box_published_products` | 公開商品snapshot | `published_products[]` | `PublicProduct`/`GuestBoxProduct` → published tables | array<object> | Yes | M/C | snapshot更新規則 | live productsを直接返さない |
| ゲスト会社 / guest company DTO / `guest_companies` | ゲスト会社 | `guest_company_id/code/name` | Go `GuestCompany` → DB同名 | object | No | M/R/C | buyers/clientCompaniesとの正本整理 | 二重管理をAPIへ露出しない |
| ゲスト認証 / credential command / `guest_credentials` | ゲストID・パスワード | `guest_login_id`, `password_secret` | form `guest_id/password` → `guest_id/password_hash` | string | No | M/C | — | read DTOにhash/passwordを含めない |

## 9. 購入依頼・取り置き

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| ゲストカート・管理一覧 / request group DTO | リクエストID / `purchaseRequests.id` | `request_group_id` | form `request_group_id` → Go field → `purchase_requests.request_group_id` | string | No | M/R/C | parent table新設要否はP1 | 現行は同値行をgroup化 |
| 同上 | 購入者 | `guest_company_id`, `guest_name` | form `guest_name`; Go `GuestCompanyID/GuestName`; row `guest_name` | string pair | Conditional | M/R/C | ゲスト会社正本 | 既存自由入力を維持 |
| 同上 | 日時 | `requested_at` | DB `requested_at` → Go `RequestedAt` | timestamp | No | M/C | M `date`の日時粒度 | dateのみへ縮退しない |
| 同上 | 備考 | `message` | form `message` → Go `Message` → DB同名 | string | Yes | M/R/C | — | M `note`から名称変換 |
| 同上 | グループ状態 | `group_status` | 行statusとreservation statusから導出 | string(enum) | No | M/R/C | **P0/P1 集約状態対応** | 一部承認を表せる必要 |
| 購入依頼明細 / item DTO / `purchase_requests` | 商品 | `product_id`, `product_code` | form `product_ids[]` → Go IDs → row `product_id` JOIN code | string pair | No | M/R/C | — | — |
| 同上 | 判定 | `decision` | approve/reject handler → `purchase_requests.status` | string(enum) | Conditional | R/C | 部分承認のgroup表示 | — |
| 取り置き / reservation DTO / `reservations` | 出荷を保留 | `reservation_status`, `starts_at`, `expires_at` | Go fields → reservation table | object | Conditional | M/R/C | 有効期限・解除・完了規則 | Mの「保留中」ラベルとCの4状態を対応 |

## 10. 承認・利用者・通知

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 承認一覧・詳細 / approval DTO / `approval_requests` | 承認ID | `approval_request_id` | Go `ID` → DB `id` | string | No | M/C | — | — |
| 同上 | 種別 | `approval_type`, `target_type`, `action_key` | Go/DB同名 | string(enum) | No | M/C | typeの正式列挙 | M `type/typeLabel`を表示分離 |
| 同上 | 申請者 | `applicant_user_id`, `applicant_name` | Go fields → `applicant_user_id` JOIN user | string pair | No | M/C | M buyerIdとの用語 | — |
| 同上 | 対象snapshot | `requested_snapshot`, `snapshot_hash` | Go/DB同名 | versioned object/string | No | C | schema version | 現行監査能力を維持 |
| 同上 | ステータス | `status_code` | DB `pending/approved/returned/rejected/cancelled/expired` | string(enum) | No | M/R/C | **P0 最終状態・再申請** | Mの3状態へ縮退しない |
| 承認操作 / action DTO / `approval_actions` | 承認・差し戻し・却下 | `action`, `comment` | form `comment` + route action → DB同名 | string enum + string | Conditional | M/R/C | P0 許可操作と必須comment | 履歴append-only維持 |
| OTP検証 / 未実装 | OTP | `otp_challenge` | Cに保存・検証なし | object | Conditional | M | **P0 OTP採否・期限・試行数・secret保存** | 未実装をAPIに見せない |
| 利用者マスタ / user DTO / `users/roles` | ユーザーID・名前・ロール | `user_id`, `display_name`, `role_codes[]` | form `username/display_name/role` → user/role tables | object | No | M/R/C | M roleとC RBAC対応 | 単一role表示と多権限を分離 |
| 認証command / `users` | パスワード | `password_secret` | form `password` → password hash | string(write-only) | Conditional | M/C | — | 平文をread DTO/DBへ保存しない |
| 通知センター / notification DTO / 未実装 | 通知 | `notifications[]` | 現行は承認・購入依頼の件数を都度導出 | array<object> | Yes | M/C | **P0 永続化・既読・生成条件** | 未実装を永続通知と誤表記しない |

## 11. 棚卸・相場・為替・ダッシュボード

| 画面/API/DB | external field | canonical DTO field | current field | 型 | nullable | source | decision / P0依存 | 互換性影響 |
|---|---|---|---|---|---|---|---|---|
| 棚卸 / stocktake DTO / `stocktakes` | 棚卸番号・実施日 | `stocktake_number`, `stocktake_date` | Go `Number/Date` → DB同名 | string + date | No | R/C | — | M永続entityなし |
| 同上 | 理論件数・金額 | `expected_count`, `expected_total_minor` | Go fields → DB同名 | int + int64 | No | R/C | 計算時点 | snapshotとして保持済み |
| 同上 | 実在・一致・差異件数 | `counted_count`, `matched_count`, `difference_count` | line集計で導出 | int | No | R/C | — | read DTO専用 |
| 棚卸明細 / stocktake line DTO / `stocktake_lines` | 商品コード・結果 | `product_id/code`, `counted_present` | barcode/form result → Go fields → DB | object | Conditional | R/C | 個数管理の要否 | 現行はpresent bool中心 |
| 同上 | 差異理由・レビュー | `difference_reason`, `review_status`, `approval_status` | Go/DB fields + approval JOIN | string(enum) | Conditional | R/C | **P0 差異承認と在庫反映** | 確定APIを決めない |
| 為替マスタ / rate DTO / `exchange_rate_snapshots` | 通貨・レート・更新日時 | `base_currency`, `quote_currency`, `rate_scaled`, `scale`, `observed_at` | form `currency/rate/observed_at` → Go `ExchangeRate` → DB | object | No | M/R/C | **P0 provider・固定時点・丸め** | M float rateからscaled整数へ変換 |
| 相場一覧 / product market DTO / `product_market_prices` | 仕入相場・売値相場 | `purchase_market_amount_minor`, `sale_market_amount_minor` | forms → `ProductMarketPrice` → DB columns | int64(minor) | Yes | R/C | CSV仕様 | M `marketPrices`専用entityなし |
| 相場履歴 / market DTO / `market_price_records` | 相場日・ブランド・型番・価格・通貨 | 同名snake_case | form → `MarketPriceInput` → DB同名 | object | No | C | 最新値と履歴の責務 | product market current値と混同しない |
| ダッシュボード / dashboard read DTO | 各集計カード | `inventory_summary`, `sales_summary`, `purchase_summary`, `request_summary` | 複数repository query / view struct | object | No | M/R/C | 集計期間・状態集合 | 保存DTOではない |
| ダッシュボード設定 / setting DTO / `master_records(category=dashboard)`候補 | 売上目標・仕入予算 | `sales_target_minor`, `purchase_budget_minor` | M `dashboardSettings`; Cは未接続候補 | int64(minor) | Yes | M/C | 設定をUIに残すか | モックにない表示は追加しない |

## 12. 画面→API用途→DBの集約マッピング

以下の「API用途」は境界名であり、パス・method・payloadを確定するものではない。

| 画面 | API用途（暫定） | 主read DTO | 主write command | 現行DB |
|---|---|---|---|---|
| 在庫一覧・詳細 | 商品検索・商品取得 | `ProductSummary`, `ProductDetail` | `UpdateProduct` | `products`, `product_images`, `inventory_events`, masters |
| 商品登録 | 商品コード候補・商品登録 | `ProductFormOptions` | `CreateProduct` | `products`, `purchase_slips/lines`, `product_images`, sequence |
| 仕入登録・伝票 | 仕入検索・詳細 | `PurchaseSlipDetail` | `Create/Update/ConfirmPurchase` | `purchase_slips`, lines, revisions, products |
| 売上登録・伝票 | 売上検索・詳細 | `SalesSlipDetail` | `Create/Update/ConfirmSale` | `sales_slips`, lines, revisions, exchange snapshot |
| 出荷登録・伝票 | 出荷検索・詳細 | `ShipmentSlipDetail` | `Create/Update/ConfirmShipment` | `shipment_slips`, lines, allocations, revisions |
| 返品/持ち帰り | 対象検索・処理詳細 | `ReturnCaseDetail` | `CreateReturn`, `RestoreInventory` | `return_takehome_items`, sales/product |
| 仕入返品 | 返品検索・詳細 | `PurchaseReturnDetail` | `Create/CompletePurchaseReturn` | purchase return header/lines |
| 請求書 | 帳票preview | `InvoicePreview` | `Issue/PrintInvoice` | source slips/lines + issued/printed metadata |
| マスタ | 分類別一覧・詳細 | `MasterRecord` | `Save/DisableMasterRecord` | `suppliers`, `master_records` |
| BOX・ゲスト | 公開matrix・公開snapshot | `BoxPublicationView`, `PublicProduct` | `AssignProduct`, `PublishSnapshot` | guest company/box/publication/snapshot tables |
| 購入依頼 | グループ一覧・詳細 | `PurchaseRequestGroup` | `DecideRequest`, `ReserveProduct` | purchase requests, reservations |
| 承認 | 承認一覧・詳細 | `ApprovalRequestDetail` | `Decide/ReapplyApproval` | approval requests/actions |
| 棚卸 | 棚卸・差異一覧 | `StocktakeDetail` | `Count/Review/CompleteStocktake` | stocktakes/lines, approvals |
| 相場・為替 | 現在値・履歴 | `ProductMarketPrice`, `ExchangeRate` | `Save/ImportMarket`, `SaveExchangeRate` | market/rate tables |
| ダッシュボード | 集計表示 | `DashboardSummary` | 原則なし | 複数集計 |

## 13. 状態値のDTO対応（未確定を含む）

| 対象 | external M/R | canonical `status_code`候補 | current C | 決定 |
|---|---|---|---|---|
| 在庫 | 在庫中・売上済・出荷済等 | Cの機械可読値を正本候補 | `purchasing/in_stock/reserved/sold/shipped/cancelled/invalid` | 表示labelだけM/Rへ合わせる |
| 仕入 | 承認待ち・差戻し等の表示あり | 未確定 | `draft/confirmed/cancelled` + approval別entity | 伝票状態と承認状態を分離 |
| 売上 | 承認待ち・差戻し・処理済 | 未確定 | `draft/pending_approval/confirmed/cancelled` | 一覧表示合成規則が必要 |
| 出荷 | 承認待ち・差戻し・処理済 | 未確定 | `draft/confirmed/cancelled` + approval | 同上 |
| 売上返品 | 未対応・処理済・差戻し等 | header未確定 | item `pending/completed/cancelled` | P0 集約単位決定まで保留 |
| 仕入返品 | 承認待ち・差戻し・処理済 | C維持候補 | `pending/returned/completed` | invoice状態と分離 |
| 購入依頼 | 未対応・対応済・保留 | group状態未確定 | item `pending/approved/rejected/cancelled/expired/sold` + reservation | 部分承認を表現 |
| 承認 | pending/approved/rejected | C維持 | `pending/approved/returned/rejected/cancelled/expired` | Mへ縮退しない |
| 棚卸 | 進行中・確定 | C維持候補 | `draft/completed` + line review/approval | 在庫反映時点をP0決定 |

## 14. P0決定台帳

| ID | 決定事項 | 影響するcanonical DTO | 未決定中の扱い |
|---|---|---|---|
| DTO-P0-01 | 通常/免税の保存単位、税率、端数 | `tax_mode`, `subtotal_minor`, `tax_minor`, `total_minor` | 現行form検証のみ。契約化しない |
| DTO-P0-02 | JPY/USDの取得元、固定時点、丸め | currency/rate snapshot | 現行DBsnapshotを保持し、UI既定を正本化しない |
| DTO-P0-03 | 売上→出荷→返品→在庫の最終状態遷移 | sales/shipment/return status | command単位で既存処理を維持 |
| DTO-P0-04 | 売上返品のheader集約単位 | `SalesReturnDetail`, number/status/date | 現行item集約read modelとして扱う |
| DTO-P0-05 | 承認OTP採否と再申請フロー | `otp_challenge`, approval state | OTP項目をAPIへ出さない |
| DTO-P0-06 | 棚卸差異承認と在庫反映 | stocktake review/finalize | 現行処理を超える契約を作らない |
| DTO-P0-07 | BOX正本、公開、差替え、履歴 | `box_code`, assignment/publication DTO | `box_text`とrelationの双方を明示 |
| DTO-P0-08 | 販売先、ゲスト会社、購入者の正本 | customer/company/buyer IDs | IDと表示snapshotを分ける |
| DTO-P0-09 | 請求書発行元、発行/印刷、帳票snapshot | `InvoicePreview/State` | ハードコード値を契約化しない |
| DTO-P0-10 | CSV列、encoding、検証、原子性 | purchase/market CSV DTO | 専用DTOを確定しない |

## 15. 互換アダプター方針

API実装時にM/R/Cを一つの構造体へ直接詰め込まず、次の変換を分離する。

```text
Mock / existing HTML form / future JSON
        ↓ input adapter
Canonical command/query（業務意味）
        ↓ application service
Repository DTO / domain model
        ↓ persistence adapter
SQLite / D1 / PostgreSQL
```

- HTMLフォーム互換:
  既存のsnake_case form名は移行期間中に受理し、canonical commandへ変換する。
- M互換:
  `code`, `id`, `price`, `date`の曖昧名は公開APIの標準名にせず、M専用view adapterで
  `product_code`, `*_slip_number`, `*_amount_minor`, `*_date`へ変換する。
- 金額:
  Mの整数をJPYと推測して永続化しない。通貨の出典がない場合はUI既定値として扱い、
  command生成前に明示させる。
- ID:
  外部業務番号と内部主キーを分離し、検索結果から内部IDを解決する。
- NULL:
  現行DBの空文字とAPIのnullableを同一視しない。adapterで空文字互換を管理する。
- 状態:
  DB code、API code、日本語labelを別レイヤーに置く。
- security:
  `organization_id`は認証contextから供給し、password/hash、内部comment、監査payloadを
  read DTOへ混入させない。
- snapshot:
  過去伝票、為替、請求書、ゲスト公開は現在マスタのlive値ではなく、必要な時点値を保持する。

## 16. API設計前の未確定事項

1. canonical field名をwire JSON名としてそのまま採用するか。
2. 金額をJSON numberで送る最大値保証を置くか、decimal stringへするか。
3. 日時精度とtimezone表記をどこまで固定するか。
4. master参照を内部ID、業務code、双方のどれで受け取るか。
5. 商品コード検索後の競合・不存在・削除済みのエラー表現。
6. sales return headerを新設するか、現行item集合のread modelに留めるか。
7. `tax_mode`と税額をheader/lineのどちらへ保持するか。
8. invoiceを都度生成するか、発行時snapshot/PDFを保持するか。
9. BOXの`products.box_text`と正規化relationの移行順。
10. purchase request groupを親テーブル化するか。
11. approval OTPを採用するか、採用時のsecret、期限、試行、ロック、監査。
12. notificationを永続化するか、業務状態から導出するか。
13. CSVを同期command、非同期job、preview-confirmのどれにするか。

## 17. 自己監査

- [x] 新規文書のみを作成し、既存コード・UI・DB・migration・APIを変更していない。
- [x] M、R、Cをsource列で区別し、RをMの確定仕様として混在させていない。
- [x] external field、canonical DTO field、current field、型、nullable、source、
      decision/P0依存、互換性影響を主要画面・API用途・DBごとに記録した。
- [x] APIパス、HTTP method、JSON契約、DB変更を確定していない。
- [x] 平文password、組織scope欠落、監査・snapshotの後退を提案していない。
- [x] 税、為替、状態遷移、承認OTP、返品集約、棚卸、BOX、請求書、CSVの未確定事項を
      P0台帳へ分離した。
- [x] Mの曖昧な`id/code/price/date`を現行内部ID・金額・日時へ無条件マッピングしていない。
