# 現行 SQLite 読み取り専用診断

- 実施日: 2026-07-30
- 対象: `.data/zaiko.db`
- 接続方式: SQLite URI `mode=ro`
- データ変更: なし
- 個別レコード値の出力: なし

## 1. 整合性

| 検査 | 結果 |
|---|---:|
| `PRAGMA integrity_check` | `ok` |
| 外部キー違反 | 0件 |
| 適用 migration | 27件 |
| 最終 migration | `000027_purchase_request_groups` |
| テーブル | 50 |
| 明示索引 | 47 |
| トリガー | 2 |
| ビュー | 0 |

## 2. 主要テーブル件数

| 分類 | テーブル | 件数 |
|---|---|---:|
| 組織・利用者 | `organizations` | 1 |
| 組織・利用者 | `users` | 6 |
| 組織・利用者 | `roles` | 2 |
| 組織・利用者 | `sessions` | 29 |
| 商品・在庫 | `products` | 12 |
| 商品・在庫 | `product_images` | 0 |
| 商品・在庫 | `inventory_events` | 28 |
| 商品・在庫 | `product_code_sequences` | 2 |
| 仕入 | `purchase_slips` | 11 |
| 仕入 | `purchase_slip_lines` | 11 |
| 売上 | `sales_slips` | 2 |
| 売上 | `sales_lines` | 2 |
| 出荷 | `shipment_slips` | 2 |
| 出荷 | `shipment_lines` | 2 |
| 返品 | `purchase_return_slips` | 9 |
| 返品 | `purchase_return_lines` | 1 |
| 返品 | `return_takehome_items` | 3 |
| 棚卸 | `stocktakes` | 1 |
| 棚卸 | `stocktake_lines` | 4 |
| 承認 | `approval_requests` | 3 |
| 承認 | `approval_actions` | 6 |
| 購入依頼 | `purchase_requests` | 1 |
| ゲスト公開 | `guest_boxes` | 10 |
| ゲスト公開 | `guest_box_publications` | 40 |
| ゲスト公開 | `guest_box_published_products` | 1 |
| 監査 | `audit_logs` | 70 |

全50テーブルの件数は診断時に取得済みであり、個人情報や認証情報の値は出力していない。

## 3. 判定

- 現時点で、baseline候補生成を妨げるSQLiteファイル破損または外部キー違反は検出されなかった。
- 本結果はデータ内容の業務整合性、状態遷移、テナント境界、金額合計の正当性を保証しない。
- 次は、列・制約・索引・トリガーのmanifest化と、M/R/C DTOマッピングに基づくP0未確定事項の分離を行う。
- 現行SQLiteへの書込み、`000028`の作成・適用、実画像移動、Cloudflareへの実データ投入は実施していない。

## 4. 実行上の制約

読み取り専用診断用Goコマンドと単体テストを追加したが、この端末ではGo SDKがPATHおよび標準的なインストール先に存在しないため、Goによるコンパイル・テストは未実行である。今回の診断値は、バンドルPythonの標準`sqlite3`を読み取り専用接続で使用して確認した。
