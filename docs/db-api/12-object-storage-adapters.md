# R2 / S3 オブジェクトストレージアダプター

## 1. 対象範囲

`dataaccess.ObjectBlobStore` の実装として、次の2プロバイダーを追加した。

- Cloudflare R2: `internal/dataaccess/r2blob`
- AWS S3: `internal/dataaccess/s3blob`

この変更はオブジェクト本体の保存・確認・読出し・削除だけを扱う。DB、API、認証、画面、
既存のメタデータ更新処理、ローカルファイル実装には変更を加えない。実環境のバケットへの
接続もこの段階では行わない。

## 2. ポートとライフサイクル

両アダプターは次の既存契約を実装する。

- `Put`: 上書き禁止でバイト列を保存し、SHA-256とサイズを返す
- `Head`: バイト列を開かず、存在、SHA-256、サイズを返す
- `Open`: ストリーミング読出しを開始する
- `Delete`: 存在しない場合も成功する冪等削除

DBメタデータとの二段階ライフサイクルは上位サービスの責務を維持する。

1. DBへ `pending` メタデータを登録する
2. R2/S3へ `Put` する
3. `Head` で存在、サイズ、SHA-256を照合する
4. DBを `ready` にする
5. 失敗時は削除を試み、DBを `failed` にする

アダプターはDBトランザクションを開始せず、DBとオブジェクトストレージをまたぐ
疑似トランザクションも作らない。

## 3. オブジェクトキーとテナント分離

保存先キーはアダプター内部で次の形に組み立てる。

```text
<prefix>/<SHA-256(tenant_id)>/<object_id>
```

- 生の `tenant_id` はプロバイダーへ送らない
- `object_id` は英数字、`_`、`-` の1〜128文字だけを許可する
- `prefix` は空文字、空セグメント、`.`、`..` を許可しない
- bucket、key、region、endpoint、署名、プロバイダー応答本文はポート外へ返さない

同じ `object_id` でもテナントが異なれば別キーとなる。テナントをまたぐ検索や一覧取得は
提供しない。

## 4. 保存時の検証

現行の画像契約に合わせ、次のContent-Typeだけを受け付ける。

- `image/jpeg`
- `image/png`
- `image/webp`

申告されたContent-Typeだけを信用せず、先頭バイトから検出した形式との一致を必須とする。
入力は `maxBytes + 1` までに制限して読み込み、空データと上限超過を拒否する。
SHA-256はアップロード前にローカル計算する。

保存時には次を送信する。

- `If-None-Match: *`: 既存キーの上書きを防止
- `x-amz-content-sha256`: SigV4のペイロードハッシュ
- `x-amz-meta-sha256`: 後続の`Head`照合用メタデータ

`Head` は `x-amz-meta-sha256` が64桁のSHA-256であること、および
`Content-Length` が非負であることを検証する。

## 5. 設定

AWS ECS本番では固定アクセスキーを渡さず、`internal/platform/awssigner`
を `s3blob.Config.Signer` に注入する。AWS SDK v2の標準credential chainが
ECS task roleの短期credentialを取得し、各リクエストでSDK cacheから最新値を
取得する。固定キー欄はCloudflare R2検証または明示的なローカル試験用であり、
AWS本番構成では使用しない。

### AWS S3

`s3blob.Config` に次を渡す。

- `Endpoint`: HTTPSのS3エンドポイント
- `Bucket`: 保存先バケット
- `Region`: SigV4リージョン
- `Prefix`: アプリケーション専用プレフィックス
- `AccessKeyID`
- `SecretAccessKey`
- `SessionToken`: 一時認証情報を使う場合のみ

### Cloudflare R2

`r2blob.Config` にR2のS3互換エンドポイント、バケット、プレフィックス、認証情報を渡す。
SigV4リージョンはR2の仕様に合わせてアダプターが `auto` に固定する。

`New` は設定を検証するだけでネットワーク接続を行わない。平文HTTPは拒否する。
`AllowInsecure` はローカルの `httptest` 用であり、本番設定では使用しない。

独自認証基盤が必要な場合は、狭い `RequestSigner` 境界を差し替えられる。標準実装は
外部SDKを導入せず、Go標準ライブラリでAWS Signature Version 4を生成する。

## 6. セキュリティ境界

- 署名済みリクエストのHTTPリダイレクトは追従しない
- エラー文字列にendpoint、bucket、key、tenant、object ID、認証情報、応答本文を含めない
- シークレットアクセスキーはAuthorizationヘッダーに直接含めない
- 呼出し側のcontextをHTTPリクエストとアップロード読込みに伝播する
- `Open` が返すボディは呼出し側が必ず閉じる
- 公開URLや署名付きURLは生成しない

認証情報は環境変数またはシークレットストアから注入し、設定ファイル、ログ、DBへ保存しない。

## 7. エラーと再試行

既存のポートエラーへの変換は次の通り。

| 状態 | 戻り値 |
|---|---|
| 不正な引数、MIME不一致、サイズ超過 | `dataaccess.ErrInvalidArgument` |
| `Put` の409/412 | `dataaccess.ErrConflict` |
| `Open` の404 | `dataaccess.ErrNotFound` |
| `Head` の404 | `BlobHead{Exists:false}` |
| `Delete` の404 | 成功 |
| context取消・期限切れ | `context.Canceled` / `context.DeadlineExceeded` |
| その他 | ロケーターを含まないプロバイダー操作エラー |

408、429、500、502、503、504とネットワーク失敗は `IsRetryable` で再試行候補と判定できる。
アダプター自身は自動再試行しない。上位層が回数上限、指数バックオフ、ジッター、context期限を
設定する。

`Put` の通信結果が不明な場合、無条件に再送しない。まず同じテナント・object IDで `Head` を行い、
期待するSHA-256とサイズが一致すれば成功として調停する。一致しない場合だけ、上位の
二段階ライフサイクルに従って失敗処理または新しいobject IDで再実行する。

## 8. テスト範囲

実バケットを使用せず、`httptest` のみで次を確認する。

- Put / Head / Open / Deleteのライフサイクル
- 上書き防止と削除の冪等性
- テナント分離と生tenant IDの非送信
- SHA-256とサイズ
- S3リージョンおよびR2 `auto` のSigV4スコープ
- 不正入力時にHTTPアクセスしないこと
- context取消
- 再試行可能エラーの分類
- エラーの機密情報除去
- 署名済みリダイレクトの禁止
- HTTPS、bucket、prefix、認証情報の設定検証

## 9. 実環境接続フェーズへの引継ぎ

実環境接続時には別作業として次を確認する。

- R2/S3のbucket、リージョン、endpoint、CORS、保持期間
- 最小権限のIAM/R2トークン
- サーバー時刻同期と認証情報ローテーション
- プロバイダー固有の条件付きPut対応
- タイムアウト、再試行回数、監視指標、アラート
- pending/ready/failed/deletedの定期照合
- 既存オブジェクトへ `x-amz-meta-sha256` がない場合の移行方針

上記が確定するまで、UI用の仮データやローカル保存を本番R2/S3処理として扱わない。
