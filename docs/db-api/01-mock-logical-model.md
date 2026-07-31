# モック論理データモデル

## 1. 目的と読み方

この文書は、2026-07-29 時点の `schema-diagram.html` に示された
`APP_DATA` を、DB・API設計前の論理モデルとして転記したものです。
現行SQLiteスキーマの再設計案やマイグレーション仕様ではありません。

情報源を混在させないため、次の記号を使用します。

- `M`: `schema-diagram.html` に明示されたモック仕様
- `R`: `inventory-system-review-2026-07-29.md` の追加・変更要求
- `C`: 現行Go実装・SQLiteマイグレーション
- `推測`: 資料に明記がなく、設計時の確認が必要な事項

型はモックの `string`、`int`、`float`、`bool`、`array`、`object`
表記を維持します。モックはNULL可否を定義していないため、PKと明示参照以外の
必須性は原則「未定」です。ここで必須性を補完してはなりません。

## 2. モック由来の主要エンティティ（M）

### 2.1 在庫・取引

| エンティティ | PK | 項目（型） | 必須（M） | 参照 | 状態 | 主な画面 | READ | WRITE |
|---|---|---|---|---|---|---|---|---|
| `inventory` | `code:string` | `brand:string`, `brandEn:string`, `model:string`, `modelEn:string`, `ref:string`, `serial:string`, `supplier:string`, `staff:string`, `purchasePrice:int`, `salePrice:int`, `purchaseDate:string`, `status:string`, `material:string`, `movement:string`, `condition:string`, `accessories:array`, `braceletQty:int`, `boxNo:int`, `images:array`, `note:string`, `comment:string`, `revisions:array` | PKのみ明示。他は未定 | `supplier→suppliers.code`, `material→materials.code`, `movement→movements.code`, `condition→conditions.code`, `boxNo→boxes.no` | 在庫中、売上済、出荷済等。完全な列挙・遷移は未定 | ダッシュボード、在庫一覧・詳細・編集、商品登録、承認、BOX、棚卸、ゲスト公開元 | ダッシュボード、在庫、商品登録、承認、BOX、棚卸 | 在庫編集、商品登録・更新、承認適用、BOX割当 |
| `sales` | `id:string` | `date:string`, `buyer:string`, `items:array<object>`, `total:int`, `note:string`, `status:string`, `pendingApprovalId:string`, `revisions:array` | PKのみ明示。他は未定 | `buyer→buyers.code`, `items[].code→inventory.code`, `pendingApprovalId→approvalRequests.id` | ステータス値・遷移は未定 | ダッシュボード、伝票一覧・売上伝票、承認、売上返品 | ダッシュボード、伝票一覧、承認 | 承認適用。画面対応表では伝票修正WRITEが欠落 |
| `sales.items[]` | 親内のキー未定 | `code:string`, `salePrice:int`, `returnType:string` | `code`は参照上必要。それ以外は未定 | `code→inventory.code` | `returnType` の値域未定 | 売上伝票詳細・修正、返品起票 | 売上伝票 | 売上登録・修正、返品起票 |
| `shipments` | `id:string` | `date:string`, `destination:string`, `items:array<object>`, `total:int`, `note:string`, `status:string`, `pendingApprovalId:string`, `revisions:array` | PKのみ明示。他は未定 | `destination→buyers.code`, `items[].code→inventory.code`, `pendingApprovalId→approvalRequests.id` | ステータス値・遷移は未定 | ダッシュボード、伝票一覧・出荷伝票、承認 | ダッシュボード、伝票一覧、承認 | 承認適用。画面対応表では伝票修正WRITEが欠落 |
| `shipments.items[]` | 親内のキー未定 | `code:string`, `brand:string`, `model:string`, `wholesale:int` | `code`は参照上必要。他は未定 | `code→inventory.code` | なし | 出荷伝票詳細・修正 | 出荷伝票 | 出荷登録・修正 |
| `purchaseSlips` | `id:string` | `date:string`, `supplier:string`, `staff:string`, `lines:array<object>`, `status:string`, `revisions:array` | PKのみ明示。他は未定 | `supplier→suppliers.code`, `lines[].code→inventory.code` | ステータス値・遷移は未定 | ダッシュボード、商品登録、伝票一覧・仕入伝票、承認 | ダッシュボード、伝票一覧 | 商品登録時追加、承認適用。画面対応表では修正WRITEが欠落 |
| `purchaseSlips.lines[]` | 親内のキー未定 | `code:string`, `sku:string`, `purchasePrice:int`, `salePrice:int`, `productDetail:object` | `code`は参照上必要。他は未定 | `code→inventory.code` | なし | 仕入登録・仕入伝票詳細・修正 | 仕入伝票 | 仕入登録・修正 |
| `purchaseReturns` | `id:string` | `date:string`, `supplier:string`, `items:array<object>`, `reason:string`, `status:string`, `createdBy:string`, `createdAt:string`, `invoicePrinted:bool` | PKのみ明示。他は未定 | `supplier→suppliers.code`, `items[].code→inventory.code` | ステータス値・遷移は未定 | 伝票一覧・仕入返品、請求書 | 伝票一覧 | 配送番号・ステータス更新 |
| `purchaseReturns.items[]` | 親内のキー未定 | `code:string`, `brand:string`, `model:string`, `ref:string`, `serial:string`, `purchasePrice:int`, `trackingNo:string` | `code`は参照上必要。他は未定 | `code→inventory.code` | 明細状態の定義なし | 仕入返品詳細 | 仕入返品 | 配送番号更新 |
| `salesReturns` | `id:string` | `date:string`, `slipId:string`, `buyer:string`, `items:array<object>`, `total:int`, `reason:string`, `note:string`, `status:string`, `createdBy:string`, `invoicePrinted:bool` | PKのみ明示。他は未定 | `slipId→sales.id`, `buyer→buyers.code`, `items[].code→inventory.code` | ステータス値・遷移は未定 | 伝票一覧・売上返品、請求書 | 伝票一覧 | ステータス更新 |
| `salesReturns.items[]` | 親内のキー未定 | `code:string`, `salePrice:int` | `code`は参照上必要。他は未定 | `code→inventory.code` | 明細状態の定義なし | 売上返品詳細 | 売上返品 | 返品起票・完了 |

### 2.2 マスタ

| エンティティ | PK | 項目（型） | 必須（M） | 参照 | 状態 | 主な画面 | READ | WRITE |
|---|---|---|---|---|---|---|---|---|
| `suppliers` | `code:string` | `name:string`, `address:string`, `contact:string`, `invoice:string` | PKのみ明示。他は未定 | `inventory.supplier`, `purchaseSlips.supplier`, `purchaseReturns.supplier` から参照 | 状態定義なし | 在庫、仕入、仕入返品、マスタ | 各参照画面 | マスタCRUD |
| `buyers` | `code:string` | `name:string`, `address:string`, `contact:string`, `invoice:string` | PKのみ明示。他は未定 | 売上・出荷・売上返品・BOX公開先・ゲストの `buyerCode` から参照 | 状態定義なし | 在庫検索、売上、出荷、ゲスト、BOX、マスタ | 各参照画面 | マスタCRUD |
| `conditions` | `code:string` | `name:string` | PKのみ明示 | `inventory.condition` | 状態定義なし | 在庫、商品、ゲスト、マスタ | 在庫・商品・ゲスト | マスタCRUD |
| `materials` | `code:string` | `name:string` | PKのみ明示 | `inventory.material` | 状態定義なし | 商品、在庫、マスタ | 商品・在庫 | マスタCRUD |
| `movements` | `code:string` | `name:string` | PKのみ明示 | `inventory.movement` | 状態定義なし | 商品、在庫、マスタ | 商品・在庫 | マスタCRUD |
| `boxes` | `no:int` | `name:string`, `publicTo:array<string>`, `createdAt:string` | PKのみ明示。他は未定 | `publicTo[]→buyers.code[]`, `inventory.boxNo→boxes.no` | 公開状態は `publishedSnapshot` 側 | BOX管理、在庫検索、商品、ゲスト | BOX、在庫、商品 | BOX編集、商品割当、公開 |
| `fxRates` | `code:string` | `name:string`, `symbol:string`, `flag:string`, `rate:float`, `updatedAt:string`, `updatedBy:string` | PKのみ明示。他は未定 | ゲスト価格表示で使用 | 履歴・有効期間なし | マスタ＞為替レート、ゲスト | マスタ、ゲスト | マスタ更新 |

### 2.3 認証・承認・購入依頼

| エンティティ | PK | 項目（型） | 必須（M） | 参照 | 状態 | 主な画面 | READ | WRITE |
|---|---|---|---|---|---|---|---|---|
| `users` | `id:string` | `role:string`, `name:string`, `loginId:string`, `email:string`, `password:string`, `avatar:string`, `approvalCode:string` | PKのみ明示。認証必須項目は未定 | 承認申請者、通知送受信者等 | `role=admin/buyer` | ログイン、マスタ＞ユーザー、承認 | 認証、マスタ、承認 | マスタCRUD |
| `guestAccounts` | `id:string` | `name:string`, `company:string`, `password:string`, `buyerCode:string`, `email:string` | PKのみ明示。認証必須項目は未定 | `buyerCode→buyers.code`, `clientCompanies.guestId` | 状態定義なし | ゲストログイン、購入依頼、ゲスト管理 | ゲスト認証、購入依頼 | マスタ／ゲスト管理 |
| `approvalRequests` | `id:string` | `buyerId:string`, `buyerName:string`, `type:string`, `typeLabel:string`, `detail:object`, `status:string`, `note:string`, `otp:string`, `otpExpiry:string`, `otpUsed:bool`, `otpAttempts:int`, `createdAt:string` | PKのみ明示。他は未定 | `buyerId→users.id`、各伝票の `pendingApprovalId` | `pending/approved/rejected`。差戻し・再申請はモック項目にない | 承認管理 | 承認管理 | OTP検証、承認適用、各対象ステータス更新 |
| `purchaseRequests` | `id:string` | `guestId:string`, `guestName:string`, `date:string`, `status:string`, `note:string`, `items:array<object>` | PKのみ明示。他は未定 | `guestId→guestAccounts.id`, `items[].itemCode→inventory.code` | 未対応、対応済、保留中 | ゲストカート・購入依頼、管理者購入一覧、ダッシュボード | 管理者一覧、ダッシュボード | ゲスト送信、管理者判定 |
| `purchaseRequests.items[]` | 親内のキー未定 | `itemCode:string`, `itemName:string`, `salePrice:int`, `itemStatus:string` | `itemCode`は参照上必要。他は未定 | `itemCode→inventory.code` | `itemStatus` 値域未定 | 購入依頼詳細 | 購入依頼 | 承認・拒否 |

### 2.4 公開スナップショット・設定・通知

| エンティティ | PK | 項目（型） | 必須（M） | 参照 | 状態 | 主な画面 | READ | WRITE |
|---|---|---|---|---|---|---|---|---|
| `publishedSnapshot` | 明示なし | `updatedAt:string`, `boxes:array<object>` | 未定 | `boxes[].no→boxes.no`, `boxes[].publicTo[]→buyers.code` | 公開時点のスナップショット | ゲスト商品一覧・詳細 | ゲストのみ | BOX管理の一括公開時のみ |
| `publishedSnapshot.boxes[]` | 親内の `no` | `no:int`, `name:string`, `publicTo:array<string>`, `items:array<object>` | `no`は参照上必要。他は未定 | `no→boxes.no`, `publicTo[]→buyers.code`, `items[].code→inventory.code` | 公開済データ | ゲスト商品一覧・詳細 | ゲスト | 公開処理 |
| `clientCompanies` | `id:string` | `guestId:string`, `companyName:string`, `representative:string`, `contactPerson:string`, `email:string`, `tel:string`, `address:string` | PKのみ明示。他は未定 | `guestId→guestAccounts.id` | 状態定義なし | ゲスト管理、取引先マスタ | ゲスト管理 | マスタCRUD |
| `companyInfo` | 明示なし | `companyName:string`, `address:string`, `tel:string`, `email:string`, `bankName:string`, `branchName:string`, `accountNumber:string`, `accountHolder:string` | 未定 | 請求書発行元 | 状態定義なし | マスタ＞自社情報、請求書 | マスタ、請求書 | マスタ更新 |
| `dashboardSettings` | 明示なし | `salesTarget:int`, `purchaseBudget:int` | 未定 | ダッシュボード | 状態定義なし | マスタ＞ダッシュボード管理、ダッシュボード | マスタ、ダッシュボード | マスタ更新 |
| `notifications` | `id:string` | `toUserId:string`, `fromUserId:string`, `type:string`, `title:string`, `body:string`, `relatedId:string`, `read:bool`, `createdAt:string` | PKのみ明示。他は未定 | `toUserId/fromUserId→users.id`, `relatedId→approvalRequests.id` | 未読／既読 | 通知センター | 通知センター | 既読更新、承認等から生成 |
| `itemNumberByDate` | `dateKey:string` | `counter:int` | PKのみ明示。他は未定 | 商品コード採番 | 状態定義なし | 商品登録、仕入登録 | 採番処理 | 採番処理 |

## 3. 画面対応（M）

`schema-diagram.html` の画面対応表を、データ境界だけに要約します。

| 画面 | READ | WRITE |
|---|---|---|
| ダッシュボード | `inventory`, `sales`, `shipments`, `purchaseSlips` | なし |
| 在庫一覧・検索 | `inventory`, `suppliers`, `buyers`, `conditions` | `inventory`（編集モーダル） |
| 商品登録 | `inventory`, `suppliers`, `boxes` | `inventory`, `purchaseSlips`, `itemNumberByDate` |
| 仕入伝票 | `purchaseSlips`, `suppliers` | 表ではなし |
| 売上伝票 | `sales`, `buyers` | 表ではなし |
| 出荷伝票 | `shipments`, `buyers` | 表ではなし |
| 売上返品 | `salesReturns`, `buyers` | `salesReturns.status` |
| 仕入返品 | `purchaseReturns`, `suppliers` | 配送番号・ステータス |
| 承認管理 | `approvalRequests`, `users` | `approvalRequests`, `inventory`, `sales`, `shipments` |
| BOX管理 | `boxes`, `inventory`, `buyers` | `boxes`, `inventory.boxNo`, `publishedSnapshot` |
| 棚卸 | `inventory`, `boxes` | 表ではなし |
| マスタ＞ユーザー | `users` | `users` |
| マスタ＞取引先 | `suppliers`, `buyers`, `clientCompanies` | 同左 |
| マスタ＞為替レート | `fxRates` | `fxRates` |
| マスタ＞自社情報 | `companyInfo` | `companyInfo` |
| 通知センター | `notifications` | `notifications.read` |
| ゲスト商品一覧・詳細 | `publishedSnapshot` | なし |
| ゲストカート | メモリ上の `cart` | なし |
| 購入依頼送信 | `guestAccounts` | `purchaseRequests` |

## 4. 7月29日追加仕様（R、モックモデルと分離）

以下はレビュー追加仕様であり、上記 `APP_DATA` の明示項目ではありません。
DB/APIへ反映する場合は別途仕様確定が必要です。

| 追加領域 | 必要な論理情報 | 状態・操作 | 確定度 |
|---|---|---|---|
| 商品・在庫 | SKU、商品コード、BOX設定・紐付け、付属品チェック選択 | 一覧・詳細・編集・全件表示 | UI要件は明示。保存形式は未確定 |
| 仕入登録 | 指定行数追加、削除後採番、商品登録、CSV入出力 | 下書き、登録、キャンセル | UI要件は明示。CSV仕様の全項目は未確定 |
| 売上登録 | 管理番号検索、JPY/USD、通常/免税 | 登録、修正 | 通貨計算・税処理は未確定 |
| 出荷 | 売上連動、配送番号 | 処理前は配送番号入力不可 | 最終連携フローは未確定 |
| 返品・持ち帰り | 商品コード検索、関連売上・出荷、在庫戻し | 選択、確定、キャンセル | 在庫更新タイミングの最終確認が必要 |
| 伝票共通 | チェック列、全件表示、修正履歴 | 承認待ち、差戻し、処理済等 | 表示要件は明示。共通状態機械は未確定 |
| 請求書 | 通常/免税、JPY/USD、小計・税・合計 | プレビュー、印刷、PDF | 税・為替計算は未確定 |
| 相場表 | 商品別仕入相場・売値相場 | CSV取込・出力、詳細・編集 | 専用CSV最終仕様は未確定 |
| 棚卸 | 棚卸ヘッダー、明細、理論在庫、実在庫、差異、理由 | 一時保存、確定、不一致確認 | 実運用は未確定 |
| 承認 | 最低限の一覧・詳細、差戻し、再申請 | pending/approved/returned/rejected等 | 最終フロー、OTP要否は未確定 |
| ゲスト | 会社・BOX別公開、権限表示 | 公開、購入依頼、取置 | BOX公開運用は未確定 |

## 5. モック資料内の矛盾・欠落

1. データフロー図のダッシュボードは `inventory/sales/shipments` のみですが、
   画面対応表には `purchaseSlips` もあります。
2. 棚卸にはカウント・差異確認ノードがある一方、画面対応表にWRITE先がありません。
3. 仕入・売上・出荷伝票は修正・返品起票を持つUIですが、画面対応表はREADのみです。
4. `salesReturns.items[]` の明細エンティティと親子キーがER図で独立定義されていません。
5. BOXの公開先は `buyers.code`、ゲスト企業は `guestAccounts/clientCompanies` で、
   公開対象会社の正本が一意ではありません。
6. `users.role` は `admin/buyer` ですが、画面文脈では管理者・作業員・ゲストが使われます。
7. `approvalRequests` はOTP項目を持ちますが、レビューでは承認最終仕様が未確定です。
8. パスワードが `string password` で表現されています。これは論理項目名であり、
   平文保存を許可する仕様ではありません。
9. `fxRates.rate` は単一値ですが、基準通貨・相手通貨・有効日時・丸め規則がありません。
10. 状態値は一部しか列挙されておらず、状態遷移、取消、削除、同時更新規則がありません。

## 6. DB/API設計へ進む前の確認事項

- 販売先・ゲスト会社・BOX公開先の正本
- 売上返品を伝票ヘッダー＋明細にするか、商品単位処理にするか
- JPY/USDの入力単位、為替取得元、固定時点、丸め、加算
- 通常・免税の保存単位と税計算
- 承認対象、差戻し・再申請、OTP要否
- 在庫状態と各伝票状態の正式な状態遷移
- 棚卸差異の承認・確定・在庫反映タイミング
- BOX公開スナップショットの生成・差替え・履歴保持
- 通知の生成契機、宛先、既読、保持期間
- 相場CSVと仕入CSVの正式フォーマット

これらは推測で補完せず、決定後に物理スキーマ・API契約へ落とし込みます。
