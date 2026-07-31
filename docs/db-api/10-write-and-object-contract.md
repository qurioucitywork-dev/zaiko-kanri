# 書込み・オブジェクト保存契約

## 1. 適用範囲

この文書は、現行 SQLite、Cloudflare 検証環境の D1/R2、最終本番の
PostgreSQL/S3 に共通する業務境界を定義する。現行 UI、Handler、SQLite Store の
接続先はまだ変更しない。

## 2. 書込み共通条件

すべての更新コマンドは次を必須とする。

- `tenant_id`
- `actor_id`
- `idempotency_key`
- UTC の `requested_at`
- 更新競合を検出する `expected_version`（既存データ更新時）

冪等性キーはテナントと操作名の中で一意に保存し、正規化した要求内容のハッシュと
同じトランザクションで記録する。同じキー・同じ要求は以前の結果を返し、同じキー・
異なる要求は `ErrIdempotencyMismatch` とする。

canonical request hashはoperation、tenant、actor、業務payloadを束縛する。
`requested_at`は各transport attemptの時刻であり、同一業務要求のretry時に変化しても
同じhashになるよう除外する。業務payload内の日時（売上日、出荷日、ready日時、
deleted日時など）は除外しない。初回の`requested_at`は冪等性記録・監査・更新時刻へ
引き続き保存する。

## 3. トランザクション境界

DB接続やトランザクションハンドルを Handler、Service、Container へ公開しない。
次をそれぞれ一つの atomic command とする。

1. 商品登録
   - 商品コード採番
   - 商品、付属品、BOX紐付け
   - 初期在庫イベント
   - 監査ログ
2. 仕入確定
   - 伝票・明細番号採番
   - ヘッダー、明細、商品
   - 在庫状態、在庫イベント
   - 監査ログ
3. 売上確定
   - 伝票番号採番
   - ヘッダー、明細、起票時為替
   - 在庫状態、在庫イベント
   - 監査ログ
4. 出荷確定
   - 売上明細との割当
   - 出荷ヘッダー、明細
   - 在庫状態、在庫イベント
   - 監査ログ
5. 返品在庫戻し
   - 返品明細の状態確認
   - 確認後コンディション
   - 在庫状態、在庫イベント
   - 返品処理状態、監査ログ

D1では一つの内部Worker API request内で一つの業務トランザクションを完結させる。
ContainerとWorker間の複数HTTPリクエストにまたがるトランザクションは禁止する。

## 4. 競合と状態遷移

- `version` の compare-and-swap 不一致は `ErrConflict`。
- UNIQUE、採番、二重割当、同時承認の競合も `ErrConflict`。
- 売上未確定の出荷、請求書未発行の売上返品完了など、前提条件不足は
  `ErrPrecondition`。
- 他テナントのIDと存在しないIDはいずれも `ErrNotFound`。
- D1障害時のSQLite fallback、D1/PostgreSQL dual-writeは禁止。

## 5. 金額・通貨

- 金額は最小通貨単位の `int64`。
- 通貨は大文字3文字のISOコード。
- 外貨ではレートを整数とscaleで保存する。
- 円換算値、外貨値、適用レート、税額、免税区分を伝票時点で固定する。
- JavaScript経由の大きな整数は文字列として転送する。

## 6. 画像の二段階保存

画像保存はDBとオブジェクトストレージが分散トランザクションを共有しない前提で、
次の順序とする。

1. DBへ `pending` メタデータを登録
2. R2/S3へストリームアップロード
3. `HEAD` で存在、サイズ、SHA-256を照合
4. DBを `ready` へ更新
5. 失敗時はオブジェクト削除を試み、DBを `failed` にする

オブジェクトのIDだけを共通境界に出し、bucket、key、region、version、signed URL、
provider request ID、credentialはadapter内部へ閉じ込める。

許可する商品画像MIMEは現段階で `image/jpeg`、`image/png`、`image/webp`。
上限は15MiB。実adapterは拡張子ではなく、Content-Type、実データ、サイズを検証する。

## 7. 孤児・欠損の回復

定期reconciliationで次を検出する。

- `pending` のまま期限切れ
- `ready` だがオブジェクトが存在しない
- オブジェクトが存在するがDBメタデータがない
- checksumまたはsize不一致
- `deleted` だがオブジェクトが残存

自動修復は削除ではなく、隔離・再試行・監査イベントを基本とする。最終削除には保持期間と
管理者承認を設ける。

## 8. 実装ファイル

- `internal/dataaccess/commands.go`
- `internal/dataaccess/write_ports.go`
- `internal/dataaccess/object_ports.go`
- `internal/objectservice/service.go`

これらはprovider-neutralな境界であり、既存画面やSQLite書込み処理をまだ切り替えない。
