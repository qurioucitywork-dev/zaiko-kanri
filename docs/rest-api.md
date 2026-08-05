# REST API v1

Base path: `/api/v1`

| 分類 | Method / Path | 主な権限 | 概要 |
|---|---|---|---|
| 認証 | `POST /auth/login`, `GET /auth/me`, `POST /auth/logout`, `POST /auth/password-reset` | ログイン以外はSession | bcrypt認証、HttpOnly Cookie、CSRF、再設定token |
| 集計 | `GET /dashboard` | `dashboard.read` | 今月の確定売上・仕入、在庫、依頼、承認、6か月推移 |
| 在庫・商品 | `GET/POST /products`, `GET/PATCH /products/{id}` | `inventory.read/write` | 在庫検索、単品登録、固定商品コード、詳細参照・編集 |
| 商品画像 | `GET/POST /products/{id}/files`, `GET /product-files/{id}` | `inventory.read/write` | 画像メタデータとlocal/S3ストレージ |
| 仕入 | `GET/POST /purchases`, `GET /purchases/{id}`, `POST /purchases/{id}/confirm` | `purchase.*` | 下書き、数量明細、確定、商品自動生成 |
| 相場表 | `GET/POST /market-prices`, `PATCH /market-prices/{id}` | `market.read/write` | 相場詳細、マスタ参照、行編集 |
| 相場CSV | `POST /market-prices/imports/preview`, `GET /market-prices/imports/{id}`, `POST .../commit` | `market.import` | 全行検証、プレビュー、承認・確定 |
| BOX | `GET /boxes`, `PUT /boxes/{code}` | `inventory.publish` | 商品・公開先企業を固定コードで関連付け |
| ゲスト | `GET /guest/catalog`, `GET/POST /guest/purchase-requests` | guest Session | 現在在庫を再照合するライブカタログと購入依頼 |
| 購入依頼 | `GET /purchase-requests`, `POST /purchase-requests/{id}/{decision}` | `request.read/review` | 承認時の排他取置、却下、競合防止 |
| 承認 | `GET/POST /approvals`, `POST /approvals/{id}/{decision}` | `approval.*` | 作業者申請と管理者承認・差戻し・却下 |
| 通知 | `GET /notifications`, `POST /notifications/{id}/read` | Session | DB共有通知と既読状態 |
| 売上 | `GET/POST /sales`, `GET /sales/{id}`, `POST /sales/{id}/confirm` | `sales.*` | 通貨・税・為替snapshotを含む売上伝票 |
| 出荷 | `GET/POST /shipments`, `GET /shipments/{id}`, `POST /shipments/{id}/confirm`, `PATCH /shipments/{id}/tracking` | `shipment.*` | 出荷伝票、在庫状態、配送会社・追跡番号 |
| 返品 | `GET/POST /returns`, `GET /returns/{id}`, `POST /returns/{id}/confirm`, `PATCH /returns/{id}/tracking` | `inventory.read/write` | 売上返品、持ち帰り、仕入返品、元伝票・配送情報 |
| 伝票 | `GET /documents` | `inventory.read` | 仕入・出荷・売上・返品の統合一覧 |
| 帳票履歴 | `GET/POST /document-events` | `inventory.read` | 印刷・ダウンロードの操作者、日時、形式、保存先履歴 |
| CSV | `GET /exports/{kind}.csv` | `inventory.read` | `inventory`, `stocktake`, `market`, `purchases`, `sales`, `shipments`, `returns`, `documents` |
| 会社・設定 | `GET/PUT /company`, `GET/PUT /settings/{key}`, `GET/POST /exchange-rates` | 読取/`settings.manage`/`market.*` | 会社・振込先の正本、目標、予算、為替履歴 |
| 利用者 | `GET/POST/PATCH /users`, password/reset各API | `users.manage` | 管理者・作業者・ゲストとパスワード管理 |
| 仕入担当者 | `GET /purchase-staff` | `inventory.read` | ログイン情報を含まない担当者コード・表示名 |
| 取引先 | `GET/POST/PATCH /partners` | 読取/`settings.manage` | 販売先・仕入先・ゲスト会社の共通ID管理 |
| マスタ | `GET/POST/PATCH /masters/{kind}` | 読取/`settings.manage` | ブランド・素材・駆動方式・コンディション・付属品 |
| 運用 | `GET /audit-logs`, `GET /email-outbox` | 管理権限 | 監査ログとメール送信待ちキュー |

更新系APIでは `/auth/me` が返す `csrfToken` を `X-CSRF-Token` headerへ付ける。認証CookieはHttpOnlyのためJavaScriptから直接読み取らない。

在庫状態は伝票確定・購入依頼承認などの業務APIだけが変更する。商品更新APIから`in_stock`、`reserved`、`shipped`、`sold`、`return_pending`、`cancelled`を直接書き換えることはできない。

エラー形式:

```json
{
  "error": {
    "code": "permission_denied",
    "message": "この操作を行う権限がありません。"
  }
}
```
