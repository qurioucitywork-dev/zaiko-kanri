# DB・API実装前 調査結果

- 調査基準日: 2026-07-30
- ステータス: 調査完了／安全な基盤作業へ進行
- 本番環境: AWS
- 検証環境: Cloudflare D1 / R2 / Containers
- 詳細資料:
  - `00-platform-architecture.md`
  - `01-mock-logical-model.md`
  - `02-difference-matrix.md`
  - `03-d1-r2-physical-design.md`
  - `04-migration-plan.md`

## 1. 現在のGoアプリ構成

現行は Go 1.26、`net/http`、Go標準テンプレート、JavaScript、必要箇所のみの
HTMXで構成されたサーバーサイドWebアプリケーションである。

- 起動: `cmd/server/main.go`
- 設定: `internal/config/config.go`
- HTTP・テンプレート: `internal/web`
- DB: `internal/database`
- DBドライバ: `modernc.org/sqlite`
- 静的ファイル・テンプレート: `go:embed`
- 画像: ローカルファイル＋DBメタデータ
- 認証: DBセッション、ハッシュ化パスワード、CSRF

`internal/web.Server` が具体型 `*database.Store` へ依存し、DB処理には複数の
`BeginTx` がある。DBの交換にはドライバ差替えではなく、業務ユースケース単位の
Repository／Transaction境界が必要である。

## 2. 現在のCloudflare実行方式

Cloudflare用設定、D1/R2 binding、Workers/Pages Functions、Wasm、
Containers、Dockerfile、Cloudflare向けCIは現行リポジトリに存在しない。
現在はローカルのネイティブGoサーバーとして動作している。

検証環境の推奨方式は次のとおり。

- Goアプリ: Cloudflare Containers
- DB: D1を専用Worker API経由で利用
- 画像: R2をS3互換API経由で利用
- Worker: 業務ユースケース専用の薄いAPI。汎用SQLプロキシは禁止

Cloudflareはテスト環境であり、本番の正本にはしない。

## 3. 現在のDB構造

SQLite migrationは `000001`〜`000027` で、商品、画像、在庫イベント、
仕入、売上、出荷、返品、棚卸、承認、予約、購入依頼、ゲスト公開、
マスタ、相場、為替、監査、セッション等を既に正規化している。

重要な現行特性:

- 主要データの `organization_id` スコープ
- TEXTのアプリケーション生成ID
- 金額の最小通貨単位整数
- 外部キー、UNIQUE、CHECK、索引
- 伝票ヘッダー／明細分離
- 修正履歴、監査ログ
- パスワードハッシュ
- ゲスト公開スナップショット

既存 migration と既存データは削除・初期化・上書きしない。

## 4. モックAPP_DATAの論理構造

モックは `inventory` を中心に、`sales`、`shipments`、`purchaseSlips`、
各返品、承認、購入依頼、利用者、取引先、BOX、公開スナップショット、
為替、通知、設定を配列・オブジェクトとして保持する。

APP_DATAは画面動作を説明する論理モデルであり、D1へ一括JSON保存する
物理モデルではない。全エンティティ、項目、型、参照、画面READ/WRITEは
`01-mock-logical-model.md` に記録した。

## 5. 現行とモックの差分

現行DBはモックより正規化・組織分離・監査・認証の面で先行しているため、
安全性を落としてAPP_DATAの形へ戻さない。モックとの名称差はDTO/ViewModelで
吸収する。

主な不足・要接続:

- 通知センターの永続モデル
- 請求書発行元と自社情報マスタの接続
- 売上の通常／免税の永続化
- 売上画面から既存通貨・為替スナップショットへの接続
- 販売先とゲスト会社の正本整理
- 売上返品のヘッダー集約単位
- BOX公開運用と公開履歴

詳細は `02-difference-matrix.md` を正本とする。

## 6. 添付資料内部の矛盾

- ダッシュボードのREAD元に `purchaseSlips` が図によって欠落
- 棚卸画面は更新操作を持つが対応表にWRITE先がない
- 仕入・売上・出荷の修正UIに対して対応表がREADのみ
- `salesReturns.items[]` の独立定義と親子キーが不足
- BOX公開先の `buyers` とゲスト会社の正本が不一致
- `users.role=admin/buyer` と画面上の管理者／作業員／ゲストが不一致
- 承認OTP項目はあるが最終フローが未確定
- 為替の基準通貨、有効時点、丸めが不足
- 返品明細、状態値、状態遷移の定義が不足

## 7. 7月29日追加仕様との差分

追加仕様のうち、画面要件は現行UIへ概ね反映済みだが、データ仕様が未確定の
項目がある。

- 通常／免税の保存単位と税計算
- JPY/USDの適用時点、丸め、為替加算
- 売上・出荷・返品・在庫の最終状態遷移
- 承認の差戻し・再申請・OTP
- 棚卸差異の承認と在庫反映
- BOX公開の差替え・履歴
- 相場CSV、仕入CSVの最終形式

これらは未確定のままDB制約やAPI契約へ固定しない。

## 8. 推奨するD1物理構造

新規D1には `000001`〜`000027` 適用後の最終構造を表すbaselineを作り、
一時テーブルとpreview seedを除外する。既存migrationは変更しない。

追加変更は `000028` 以降として、テナント境界、R2メタデータ、付属品、
購入依頼グループ等を論理変更単位に分離する。金額は整数、日時はUTC、
状態・通貨・真偽値はCHECK、業務番号は組織内UNIQUEを原則とする。

## 9. 推奨するR2保存構造

バイナリはR2、所有・状態・表示順・検証属性はD1へ保存する。

推奨キー:

`{environment}/{organization_id}/products/{product_id}/{image_id}/original.{extension}`

アップロードは `pending → PUT → 検証 → ready` の二段階処理とし、
checksum、MIME、サイズを検証する。失敗・期限超過・孤児オブジェクトを
再試行・監査できる構造にする。URLは永続化しない。

## 10. GoとD1/R2の接続方式

ドメイン／サービスは次のポートへ依存する。

- Repository／Unit of Work
- Transaction runner
- ObjectStore
- MediaReader／Signed URL
- Clock
- ID generator
- Idempotency store

環境別アダプター:

- ローカル: SQLite＋filesystem
- Cloudflareテスト: D1 Worker API＋R2
- AWS本番: RDS for PostgreSQL＋S3

## 11. 作成予定のmigration

- D1 baseline: legacy `000001`〜`000027` の最終形
- `000028`以降の候補:
  - テナント境界強化
  - R2画像メタデータ
  - 付属品正規化
  - 商品属性マスタ参照
  - 購入依頼グループ親
  - JSONスキーマ版
  - 冪等リクエスト／非同期処理状態

番号と内容は現行データ診断、D1 capability test、P0仕様確定後に固定する。

## 12. 実装順序

1. 現行データの読取専用診断
2. Repository／Storage契約とローカルadapterの抽出
3. D1 capability test
4. D1 baseline生成・比較
5. R2/S3共通ObjectStore契約
6. D1 Worker APIとD1 adapter
7. データ変換・照合ツール
8. Cloudflare検証環境で結合試験
9. PostgreSQL adapter・AWS staging
10. AWS本番移行計画とリハーサル

## 13. 既存データへの影響

現段階は設計文書のみで、DB・画像・既存コードへの変更はない。
以後も初期作業は読取専用診断と追加ファイルを中心に行い、
既存migration、既存SQLite、既存画像を原本として保全する。

## 14. 未確定事項

- 税と免税の業務仕様
- 為替取得・固定・加算・丸め
- 承認／OTP最終仕様
- 売上・出荷・返品の状態遷移
- 棚卸差異の確定運用
- BOX公開運用
- CSV仕様
- AWS PostgreSQLのメジャーバージョン、接続方式、RTO/RPO
- セッション／CSRFの本番保存方式
- 監査保持期間

## 15. 作業上のリスク

- `*database.Store` への広い具体依存を一括変更すると回帰範囲が大きい
- D1はローカルSQLiteとtransaction・DDL・制約挙動が同一ではない
- D1とR2に分散transactionがなく、中間障害の回復が必要
- テナント境界を親参照だけに依存するテーブルがある
- APP_DATAと現行DBの正本・状態モデルが一部異なる
- P0仕様を推測で固定すると後続migrationが破壊的になる
- dirty worktreeの既存UI変更を上書きしない配慮が必要
- Cloudflare固有設計を本番AWSへ流用すると再移行コストが増える

## 進行判断

以下はP0仕様未確定でも安全に開始できる。

- 現行データの読取専用診断
- DB／Storageポートの契約定義
- SQLite／filesystem adapterの契約テスト
- D1 capability test
- legacy最終スキーマからのbaseline生成・差分検証
- R2/S3共通ObjectStoreの技術検証

税、為替、承認OTP、状態遷移、棚卸反映、BOX公開運用に依存する
schema/APIの確定実装は、仕様決定まで保留する。
