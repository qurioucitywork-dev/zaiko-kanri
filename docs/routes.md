# ルート一覧（フェーズ1〜8）

| Method | Path | 用途 | 権限 |
|---|---|---|---|
| GET | `/healthz` | ヘルスチェック | 公開 |
| GET/POST | `/login` | ログイン | 公開 |
| POST | `/logout` | ログアウト | 認証済み |
| GET | `/` | ダッシュボード | `dashboard.read` |
| GET | `/users` | 利用者一覧 | `users.manage` |
| GET/POST | `/settings` | 組織設定 | `settings.manage` |
| GET | `/audit` | 監査ログ | `audit.read` |
| GET | `/public/products` | ゲスト向け公開商品 | 公開 |
| POST | `/public/products/{id}/purchase-requests` | ゲスト購入依頼送信 | 公開・ワンタイムCSRF |
| GET | `/products` | 在庫一覧・検索・状態フィルタ | `inventory.read` |
| GET | `/products/export.csv` | 検索条件に一致する在庫・原価CSV出力 | `inventory.read` |
| GET/POST | `/products/new`, `/products` | 商品単品登録 | `inventory.write` |
| GET | `/products/{id}` | 商品詳細・履歴 | `inventory.read` |
| POST | `/products/{id}/status` | 検品完了・在庫状態変更 | `inventory.write` |
| POST | `/products/{id}/images` | 商品画像登録 | `inventory.write` |
| POST | `/products/{id}/publication` | 商品の公開・非公開変更 | `inventory.publish` |
| GET | `/product-images/{id}` | 組織スコープ付き画像表示 | `inventory.read` |
| GET | `/purchases` | 仕入伝票一覧 | `purchase.read` |
| GET/POST | `/purchases/new`, `/purchases` | 仕入伝票下書き登録 | `purchase.write` |
| GET | `/purchases/{id}` | 仕入伝票詳細 | `purchase.read` |
| POST | `/purchases/{id}/confirm` | 仕入確定・商品生成 | `purchase.confirm` |
| GET | `/market-prices` | 相場・為替の一覧 | `market.read` |
| POST | `/market-prices` | 相場情報の手入力 | `market.write` |
| POST | `/exchange-rates` | 為替レートのスナップショット登録 | `market.write` |
| GET | `/market-prices/import` | 相場CSV取込画面 | `market.import` |
| POST | `/market-prices/import/preview` | CSV全行検証・プレビュー保存 | `market.import` |
| POST | `/market-prices/import/{id}/commit` | CSV取込確定または承認申請 | `market.import` |
| GET | `/sales` | 売上伝票一覧 | `sales.read` |
| GET/POST | `/sales/new`, `/sales` | 売上伝票の下書き登録 | `sales.write` |
| GET | `/sales/{id}` | 売上伝票詳細・出荷進捗 | `sales.read` |
| POST | `/sales/{id}/confirm` | 売上確定・価格為替固定 | `sales.confirm` |
| POST | `/sales/{id}/cancel` | 管理者は売上取消、作業者は承認申請 | `sales.write` |
| GET | `/shipments` | 出荷伝票一覧 | `shipment.read` |
| GET/POST | `/shipments/new`, `/shipments` | 出荷伝票の下書き登録 | `shipment.write` |
| GET | `/shipments/{id}` | 出荷伝票詳細・売上関連 | `shipment.read` |
| POST | `/shipments/{id}/confirm` | 出荷確定・数量割当 | `shipment.confirm` |
| POST | `/shipments/{id}/cancel` | 管理者は出荷取消、作業者は承認申請 | `shipment.write` |
| GET | `/purchase-requests` | 購入依頼・取置一覧 | `request.read` |
| POST | `/purchase-requests/{id}/approve` | 購入依頼承認・取置作成 | `request.review` |
| POST | `/purchase-requests/{id}/reject` | 購入依頼却下・取置解除 | `request.review` |
| POST | `/purchase-requests/{id}/cancel` | 購入依頼取消・取置解除 | `request.review` |
| GET | `/approvals` | 承認案件・判断履歴一覧 | `approval.read` |
| POST | `/approvals/{id}/approve` | 承認・対象操作実行 | `approval.approve` |
| POST | `/approvals/{id}/return` | コメント必須の差戻し | `approval.approve` |
| POST | `/approvals/{id}/reject` | 承認申請の却下 | `approval.approve` |
