# モック・現行DB/API・7月29日追加仕様 差分マトリクス

## 1. 判定基準

| 判定 | 意味 |
|---|---|
| 一致 | 論理的に同じ責務が現行にある |
| 構造差（維持） | 現行の正規化・セキュリティ・組織分離がモックより安全で、戻すべきでない |
| 現行先行 | モックにないが現行機能に必要で、削除対象ではない |
| 不足 | モックまたは確定済み追加仕様に対し、保存・読取・APIが不足 |
| 要接続 | 保存先はあるが画面・帳票が参照していない |
| 要決定 | レビューで未確定、または資料間に矛盾があり実装不可 |
| 文書差 | 実装はあるが図・ルート文書が追随していない |

「次アクション」は設計上の提案であり、この文書作成時点では実装していません。

## 2. エンティティ・機能差分

| 領域 | モック（M） | 現行DB/API（C） | 7月29日追加仕様（R） | 判定 | 次アクション／未確定 |
|---|---|---|---|---|---|
| 在庫 | `inventory` 1配列。コードPK、価格、状態、マスタ参照、画像・履歴を内包 | `products`, `product_images`, `inventory_events`, `serial_duplicate_overrides` に正規化。組織スコープ・論理削除あり | SKU、商品コード、BOX、付属品、一覧・詳細・編集統一 | 構造差（維持）／概ね一致 | 現行を正本にする。モック名とのAPI DTO対応表を作る |
| 商品コード採番 | `itemNumberByDate(dateKey,counter)` | `product_code_sequences(organization_id,purchase_date,last_sequence)` | 自動採番、行削除後の表示番号再採番 | 一致／現行が堅牢 | 組織＋日付単位の現行方式を維持 |
| 画像 | `inventory.images[]` URL配列 | `product_images` と組織スコープ付き配信 | 商品詳細・編集で統一 | 構造差（維持） | URL配列へ戻さない |
| 仕入先 | `suppliers` | 専用 `suppliers` テーブル | 全画面でマスタを正本化 | 一致 | 現行専用テーブルを維持 |
| ブランド・素材・駆動・条件・付属品 | 個別配列／マスタ | 汎用 `master_records(category)` | マスタ値を全画面の選択肢に使用 | 構造差（維持） | カテゴリ値と画面入力値の参照漏れを別途検査 |
| 販売先 | `buyers` | `master_records(category=sales-destinations)` | 売上・出荷・返品・ゲストで共通利用 | 一致に見えるが二重管理 | `guest_companies` との正本を決定 |
| ゲスト会社 | `guestAccounts`＋`clientCompanies` | `guest_companies`＋`guest_credentials` | BOX表示・権限・詳細統一 | 構造差（維持） | 現行のハッシュ認証を維持。販売先との同期方法は要決定 |
| BOX | `boxes`, `inventory.boxNo`, `publicTo[]` | `guest_boxes`, `guest_box_products`, `guest_box_drafts`, `guest_box_publications`。商品にも `box_text` | BOX一覧・登録・編集・商品紐付け | 構造差（維持）／現行先行 | `products.box_text` と `guest_box_products` の責務、公開運用を確定 |
| 公開スナップショット | `publishedSnapshot.boxes[].items[]` | `guest_box_published_products` と `guest_box_published_images` に公開時の商品情報を複製 | ゲストは公開済みBOXのみ表示 | 構造差（維持）／一致 | 現行スナップショットを維持。再公開・履歴保持方針は要決定 |
| 仕入伝票 | `purchaseSlips.lines[]` | `purchase_slips`, `purchase_slip_lines`, `purchase_slip_revisions` | SKU、商品コード、行数追加、CSV、詳細、修正履歴 | 一致／現行先行 | モック対応表のWRITE欠落を文書修正 |
| 売上伝票 | `sales.items[]` | `sales_slips`, `sales_lines`, `sales_slip_revisions` | 管理番号検索、JPY/USD、通常/免税 | 部分一致／不足 | DBは通貨を保持できるがUI登録はJPY固定。税区分は保存されない |
| 売上通貨 | `salePrice`のみ。`fxRates`をゲスト表示に使用 | `sales_lines.sale_currency` と為替スナップショット・換算額あり | 円・ドル切替 | 現行先行だが要決定 | 画面からはJPY固定。為替API・固定時点・丸め決定後に接続 |
| 通常・免税 | モック論理モデルに保存項目なし | UIで `tax_mode` を検証するがDBへ保存しない | 通常・免税切替、請求書表示 | 不足／要決定 | 伝票単位か明細単位か、税率・計算・保存値を決定 |
| 出荷伝票 | `shipments.items[]` | `shipment_slips`, `shipment_lines`, `sales_shipment_allocations`, `shipment_slip_revisions` | 売上連動、配送番号、処理前入力不可 | 現行先行／部分一致 | 売上・出荷・返品の最終フローは要決定 |
| 配送番号 | 仕入返品明細の `trackingNo` | 出荷は `shipment_slips.tracking_number`、仕入返品はヘッダー `delivery_number` と明細 `tracking_no` | 処理済み前は入力不可 | 構造差／要決定 | ヘッダー／明細どちらが正本かを伝票種別ごとに定義 |
| 売上返品 | `salesReturns` ヘッダー＋`items[]` | `return_takehome_items` が売上明細単位で返品／持帰りを保持 | 伝票一覧、請求書、在庫戻し、関連表示 | 構造差／要決定 | ヘッダーを追加するか、現行を集約表示するか決定 |
| 仕入返品 | `purchaseReturns.items[]` | `purchase_return_slips`, `purchase_return_lines` | 詳細、請求書、配送番号、完了 | 一致／現行先行 | 請求書任意完了の現行ルールと最終業務ルールを確認 |
| 請求書状態 | `invoicePrinted:bool` | 発行・印刷の日時と実行ユーザーを保持 | プレビュー、印刷、PDF、税・通貨 | 構造差（維持）／要接続 | boolへ戻さない。自社情報と税・通貨表示を接続 |
| 自社情報 | `companyInfo` | `master_records(category=company)` に保存可能 | 請求書宛先・振込情報 | 要接続 | 請求書は現在固定値。会社マスタ読取へ切替が必要 |
| 為替 | `fxRates(code,rate,updatedAt,updatedBy)` | `exchange_rate_snapshots`。USD/EUR/HKD/CHF→JPY、観測日時・提供元・更新者あり | 設定画面へ移動、売上・請求書・ゲスト利用 | 構造差（維持）／要決定 | API取得元、固定時点、加算、丸めが未確定 |
| 相場 | モックERに専用エンティティなし | `market_price_records`, import batch/rows に加え `product_market_prices` | 独立相場表、商品別仕入・売値相場、CSV | 現行先行／構造重複 | 履歴相場と商品現在値の責務、CSV最終仕様を確定 |
| 購入依頼 | `purchaseRequests.items[]` | 商品単位 `purchase_requests`＋`request_group_id`、`reservations` | 一覧・ポップアップ・承認／拒否／保留・出荷へ | 構造差（維持）／現行先行 | グループをAPI上の伝票単位DTOとして扱う |
| 取置 | 購入依頼状態「保留中」 | `reservations` で開始・期限・解除・完了を保持 | 購入依頼・取り置き | 現行先行 | 状態対応表を定義 |
| 承認 | `approvalRequests`、対象伝票の `pendingApprovalId`、OTP | `approval_requests`, `approval_actions`。対象型＋ID＋スナップショット＋実行payload | 最低限UI、承認、差戻し、再申請 | 構造差（維持）／要決定 | OTP有無と最終フロー未確定。現行監査可能構造を維持 |
| ユーザー | `users(role=admin/buyer,password)` | `users`, `roles`, `permissions`, `user_permissions`, `sessions`。パスワードハッシュ | 権限表示・マスタ管理 | 構造差（維持） | 平文passwordへ戻さない。role名称対応を確定 |
| 通知 | `notifications`、既読更新 | 専用テーブル・通知センタールートなし。ベル件数は承認と購入依頼から集計 | レビューで明確な追加仕様なし | 不足／要決定 | 通知センターを実装対象に含めるか先方確認 |
| パスワード通知 | モックの通知とは別定義なし | 外部メール未接続。受付結果を監査ログに記録 | マスタ操作 | 現行の意図的未接続 | メールAPIは別フェーズ。送信済みと誤表示しない |
| 棚卸 | データフロー上は在庫読取のみ。永続エンティティなし | `stocktakes`, `stocktake_lines`。一時保存、差異理由、レビュー、確定あり | 棚卸登録・集計・不一致リスト | 現行先行／資料矛盾 | 実運用・差異承認・在庫反映時点を確定 |
| ダッシュボード設定 | `dashboardSettings` | `master_records(category=dashboard)` に保存可能 | モック一致では不要情報を除外 | 要接続または不要 | 現行ダッシュボードは設定目標を表示しない。保持要否を確認 |
| 監査 | モックERに独立定義なし | 変更不可トリガー付き `audit_logs` | 7月29日時点では優先度C | 現行先行（維持） | UI非表示判断とDB保持を混同しない |
| 組織分離 | モックに組織キーなし | 主要テーブルに `organization_id` | 組織設定は優先度C | 構造差（維持） | 組織スコープを削除しない |

## 3. 画面READ/WRITE差分

| 画面 | モック対応表 | 現行 | 判定 |
|---|---|---|---|
| ダッシュボード | 在庫・売上・出荷・仕入をREAD | 同等データをDB集計 | 一致。モックのデータフロー図だけ仕入を欠落 |
| 在庫 | 在庫と各マスタをREAD、在庫をWRITE | 一覧・CSV・詳細・モーダル・編集・画像・公開状態APIあり | 現行先行 |
| 商品登録 | 在庫・仕入伝票・採番をWRITE | 単品登録時に仕入伝票・明細・商品を生成 | 一致 |
| 仕入伝票 | READのみ | 登録・確定・編集・返品起票・CSVあり | 文書差 |
| 売上伝票 | READのみ | 登録・確定・取消・編集・返品起票・請求書あり | 文書差 |
| 出荷伝票 | READのみ | 登録・確定・取消・編集あり | 文書差 |
| 売上返品 | ステータスWRITE | 起票・請求書・完了・在庫戻しあり | 現行先行／モデル差 |
| 仕入返品 | 配送番号・ステータスWRITE | 起票・請求書・完了・配送番号あり | 一致／現行先行 |
| 承認 | 承認と対象状態をWRITE | 汎用承認、アクション履歴、対象操作payloadあり | 構造差（維持） |
| BOX | BOX・在庫boxNo・公開スナップショットをWRITE | BOX商品紐付けと会社別公開スナップショットをWRITE | 構造差（維持） |
| 棚卸 | READのみ | 作成・明細更新・一時保存・確定をWRITE | モック資料矛盾／現行先行 |
| 通知センター | READ・既読WRITE | 画面・永続化なし | 不足 |
| ゲスト | 公開スナップショットREAD、購入依頼WRITE | 会社別公開スナップショットREAD、グループ購入依頼WRITE | 構造差（維持）／一致 |

## 4. 状態差分

| 対象 | モック | 現行DB | 判定 |
|---|---|---|---|
| 在庫 | 日本語表示相当のみ | `purchasing/in_stock/reserved/sold/shipped/cancelled/invalid` | API・UI表示マッピングを正本化 |
| 仕入 | 値域未定 | `draft/confirmed/cancelled` | 現行先行 |
| 売上 | 値域未定 | `draft/pending_approval/confirmed/cancelled` | 現行先行 |
| 出荷 | 値域未定 | `draft/confirmed/cancelled` | 現行先行 |
| 購入依頼 | 未対応／対応済／保留中 | `pending/approved/rejected/cancelled/expired/sold`＋予約状態 | 要対応表 |
| 承認 | `pending/approved/rejected` | `pending/approved/returned/rejected/cancelled/expired` | 現行先行。再申請ルール未確定 |
| 返品・持帰り | 値域未定 | `pending/completed/cancelled` | 要対応表 |
| 仕入返品 | 値域未定 | `pending/returned/completed` | 要対応表 |
| 棚卸 | モックに保存状態なし | `draft/completed`、明細レビュー状態あり | 現行先行 |

DB値をモックの日本語ラベルへ直接置換せず、DBの機械可読値とUI表示ラベルを
分離する必要があります。

## 5. 優先度別の結論

### P0: 仕様決定までDB/API変更禁止

1. 為替API、固定時点、丸め、加算方法
2. 通常・免税の保存単位と税計算
3. 承認最終フローとOTP
4. 売上・出荷・返品の最終連携
5. 棚卸の実運用と在庫反映
6. BOX公開運用
7. 相場CSV最終形式

### P1: 次の接続設計候補

1. 自社情報マスタを請求書へ接続
2. 販売先マスタとゲスト会社の正本統一
3. 売上返品のAPI集約単位を確定
4. モック名と現行DB名のDTO対応表を作成
5. 状態値・表示ラベル・許可遷移表を作成

### P2: 文書更新

- `docs/routes.md` は返品、伝票モーダル、マスタ、ゲスト管理、棚卸等の現行ルートを
  網羅していないため、サーバールートから再生成または更新する。
- `schema-diagram.html` のREAD/WRITE矛盾を、実DB設計確定時に修正する。

## 6. 根拠

- モック論理モデル:
  `C:\Users\qurio\OneDrive\Desktop\在庫管理DB\schema-diagram.html`
- 追加仕様:
  `C:\Users\qurio\Downloads\inventory-system-review-2026-07-29.md`
- 物理スキーマ:
  `internal/database/migrations/000001_phase1.up.sql` ～
  `internal/database/migrations/000027_purchase_request_groups.up.sql`
- ルート:
  `internal/web/server.go`
- 売上登録:
  `internal/web/sales_handlers.go`
- 請求書:
  `internal/web/invoice_handlers.go`
- マスタ:
  `internal/database/masters.go`, `internal/web/master_handlers.go`
- 在庫、売上、返品、棚卸:
  `internal/database/inventory.go`, `internal/database/sales.go`,
  `internal/database/returns.go`, `internal/database/stocktakes.go`

## 7. 自己監査

- [x] 既存ファイルを変更していない
- [x] UI、DB、API、マイグレーションを変更していない
- [x] モック仕様と7月29日追加仕様を別節・別列で管理した
- [x] モックにない必須性・状態遷移を推測で確定していない
- [x] 平文パスワードやOTPを現行DBへ移す提案をしていない
- [x] 現行の組織スコープ、正規化、監査、ハッシュ認証を維持対象とした
- [x] 未確定事項をP0として実装対象から除外した
